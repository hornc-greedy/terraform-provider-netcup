// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// ObjectKind is the path of a storage below a user.
type ObjectKind string

// The storages of a user.
const (
	ObjectKindImages ObjectKind = "images"
	ObjectKindIsos   ObjectKind = "isos"
)

// S3 allows 10000 parts of at least 5 MiB. 64 MiB is a size of our choice.
const (
	maxUploadParts = 10000
	minPartSize    = 64 << 20
)

// S3Object mirrors the S3Object schema.
type S3Object struct {
	Key          string `json:"key"`
	LastModified string `json:"lastModified"`
	SizeInB      *int64 `json:"sizeInB"`
	Type         string `json:"type"`
}

// S3CompletedPart mirrors the S3CompletedPart schema.
type S3CompletedPart struct {
	ETag       string `json:"ETag"`
	PartNumber int    `json:"partNumber"`
}

func (c *Client) objectsPath(ctx context.Context, kind ObjectKind) (string, error) {
	path, err := c.userPath(ctx)
	if err != nil {
		return "", err
	}
	return path + "/" + string(kind), nil
}

// ListObjects returns the objects of a storage.
func (c *Client) ListObjects(ctx context.Context, kind ObjectKind) ([]S3Object, error) {
	path, err := c.objectsPath(ctx, kind)
	if err != nil {
		return nil, err
	}
	var objects []S3Object
	if err := c.do(ctx, http.MethodGet, path, nil, &objects); err != nil {
		return nil, err
	}
	return objects, nil
}

// GetObjectDownload returns the download of an object.
func (c *Client) GetObjectDownload(ctx context.Context, kind ObjectKind, key string) (*S3DownloadInfos, error) {
	path, err := c.objectsPath(ctx, kind)
	if err != nil {
		return nil, err
	}
	var info S3DownloadInfos
	if err := c.do(ctx, http.MethodGet, path+"/"+url.PathEscape(key), nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// DeleteObject deletes an object.
func (c *Client) DeleteObject(ctx context.Context, kind ObjectKind, key string) error {
	path, err := c.objectsPath(ctx, kind)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, path+"/"+url.PathEscape(key), nil, nil)
}

// uploadPartSize returns the size of the parts of a file.
func uploadPartSize(size int64) int64 {
	return max(minPartSize, (size+maxUploadParts-1)/maxUploadParts)
}

// UploadObject uploads a file as a multipart upload.
func (c *Client) UploadObject(ctx context.Context, kind ObjectKind, key, source string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	base, err := c.objectsPath(ctx, kind)
	if err != nil {
		return err
	}
	base += "/" + url.PathEscape(key)
	var upload struct {
		UploadID string `json:"uploadId"`
	}
	if err := c.do(ctx, http.MethodPost, base+"?multipart=true", nil, &upload); err != nil {
		return err
	}
	base += "/" + url.PathEscape(upload.UploadID)

	size, partSize := info.Size(), uploadPartSize(info.Size())
	parts := []S3CompletedPart{}
	for number, offset := 1, int64(0); number == 1 || offset < size; number, offset = number+1, offset+partSize {
		etag, err := c.uploadPart(ctx, base, number, file, offset, min(partSize, size-offset))
		if err != nil {
			return fmt.Errorf("part %d: %w", number, err)
		}
		parts = append(parts, S3CompletedPart{ETag: etag, PartNumber: number})
	}
	// Assembling a big object takes long, and a resend finds no upload.
	ctx = context.WithValue(context.WithValue(ctx, sendOnceKey{}, true), noClientTimeoutKey{}, true)
	return c.do(ctx, http.MethodPut, base, parts, nil)
}

// uploadPart signs and sends a part, as often as the client retries a request.
func (c *Client) uploadPart(ctx context.Context, base string, number int, file io.ReaderAt, offset, length int64) (string, error) {
	delay := c.RetryDelay
	for attempt := 1; ; attempt++ {
		var signed struct {
			URL string `json:"url"`
		}
		if err := c.do(ctx, http.MethodGet, base+"/parts/"+strconv.Itoa(number), nil, &signed); err != nil {
			return "", err
		}
		status, etag, err := c.putPart(ctx, signed.URL, io.NewSectionReader(file, offset, length))
		if err == nil {
			return etag, nil
		}
		rejected := status != 0 && status < http.StatusInternalServerError
		if attempt == maxAttempts || ctx.Err() != nil || rejected {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// s3Error is the error S3 answers with. Only its code is safe to show, as other
// fields can echo signing data.
type s3Error struct {
	Code string `xml:"Code"`
}

// putPart sends a part to its presigned URL and returns the ETag. The client
// timeout does not fit a part, so ctx alone bounds it.
func (c *Client) putPart(ctx context.Context, target string, part *io.SectionReader) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, part)
	if err != nil {
		return 0, "", redactURL(err)
	}
	req.ContentLength = part.Size()
	if part.Size() == 0 {
		req.Body = http.NoBody
	}
	client := *c.HTTPClient
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", redactURL(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		var answer s3Error
		_ = xml.NewDecoder(resp.Body).Decode(&answer)
		return resp.StatusCode, "", fmt.Errorf("upload failed with http %d %s", resp.StatusCode, answer.Code)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return resp.StatusCode, "", errors.New("upload answered without an ETag")
	}
	return resp.StatusCode, etag, nil
}

// redactURL drops the URL of an error: a presigned URL grants access.
func redactURL(err error) error {
	var transport *url.Error
	if errors.As(err, &transport) {
		return transport.Err
	}
	return err
}
