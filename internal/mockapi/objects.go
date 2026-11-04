// SPDX-License-Identifier: MPL-2.0

package mockapi

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	s3Prefix    = "/s3/"
	maxParts    = 10000
	minPartSize = 5 << 20
)

// mockObject is an object of a user storage.
type mockObject struct {
	Key          string `json:"key"`
	LastModified string `json:"lastModified"`
	SizeInB      int64  `json:"sizeInB"`
	Type         string `json:"type"`
}

// mockUpload is a multipart upload in progress.
type mockUpload struct {
	kind, key string
	sizes     map[int]int64
}

// objectKey decodes the key, which the router leaves escaped.
func objectKey(c *call) string {
	key, err := url.PathUnescape(c.params["key"])
	if err != nil {
		return c.params["key"]
	}
	return key
}

// objectHandlers are the handlers of one storage.
func (s *Server) objectHandlers(kind string) map[string]handler {
	base := "/api/v1/users/{userId}/" + kind
	return map[string]handler{
		"GET " + base: func(c *call) (int, any) {
			s.checkUser(c)
			objects := []mockObject{}
			for _, o := range s.scp.objects[kind] {
				objects = append(objects, *o)
			}
			slices.SortFunc(objects, func(a, b mockObject) int { return strings.Compare(a.Key, b.Key) })
			return http.StatusOK, objects
		},
		"POST " + base + "/{key}": func(c *call) (int, any) {
			s.checkUser(c)
			if c.query.Get("multipart") != "true" {
				s.t.Errorf("upload without multipart=true: %v", c.query)
			}
			// Assumption: netcup refuses an existing key when the upload starts.
			if _, ok := s.scp.objects[kind][objectKey(c)]; ok {
				return badRequest(fmt.Sprintf("Image with name '%s' already exists.", objectKey(c)))
			}
			id := fmt.Sprintf("upload-%d", s.scp.newID())
			s.scp.uploads[id] = &mockUpload{kind: kind, key: objectKey(c), sizes: map[int]int64{}}
			return http.StatusCreated, map[string]string{"uploadId": id}
		},
		"GET " + base + "/{key}": func(c *call) (int, any) {
			s.checkUser(c)
			if _, ok := s.scp.objects[kind][objectKey(c)]; !ok {
				return notFound()
			}
			return http.StatusOK, map[string]any{
				"filename": objectKey(c), "presignedUrl": s.URL + s3Prefix + objectKey(c) + "?signature=mock",
				"presignedUrlValidityDurationInHours": 1, "headers": map[string][]string{"x-amz-te": {"trailers"}},
			}
		},
		"DELETE " + base + "/{key}": func(c *call) (int, any) {
			s.checkUser(c)
			// Assumption: an unknown key is no error, the spec lists only 204.
			delete(s.scp.objects[kind], objectKey(c))
			return http.StatusNoContent, nil
		},
		"GET " + base + "/{key}/{uploadId}/parts/{partNumber}": func(c *call) (int, any) {
			s.checkUser(c)
			if _, ok := s.scp.uploads[c.params["uploadId"]]; !ok {
				s.t.Errorf("part of unknown upload %s", c.params["uploadId"])
			}
			return http.StatusOK, map[string]string{"url": s.URL + s3Prefix + c.params["uploadId"] + "/" + c.params["partNumber"] + "?signature=mock"}
		},
		"PUT " + base + "/{key}/{uploadId}": func(c *call) (int, any) {
			s.checkUser(c)
			return s.completeUpload(c, kind)
		},
	}
}

func (s *Server) completeUpload(c *call, kind string) (int, any) {
	time.Sleep(s.scp.completeDelay)
	upload, ok := s.scp.uploads[c.params["uploadId"]]
	if !ok || upload.kind != kind || upload.key != objectKey(c) {
		return notFound()
	}
	var parts []struct {
		ETag       string `json:"ETag"`
		PartNumber int    `json:"partNumber"`
	}
	if err := c.decode(&parts); err != nil {
		return notFound()
	}
	if len(parts) != len(upload.sizes) {
		s.t.Errorf("completed %d parts, uploaded %d", len(parts), len(upload.sizes))
	}
	if len(parts) > maxParts {
		s.t.Errorf("%d parts, S3 allows %d", len(parts), maxParts)
	}
	var size int64
	for i, part := range parts {
		if part.PartNumber != i+1 || part.ETag != partETag(part.PartNumber) {
			s.t.Errorf("part %d = %+v", i+1, part)
		}
		if i < len(parts)-1 && upload.sizes[part.PartNumber] < minPartSize {
			s.t.Errorf("part %d has %d bytes, S3 wants %d", i+1, upload.sizes[part.PartNumber], minPartSize)
		}
		size += upload.sizes[part.PartNumber]
	}
	// Assumptions: an upload replaces an existing key, and its type is MANUAL.
	s.scp.objects[kind][upload.key] = &mockObject{Key: upload.key, LastModified: snapshotCreated, SizeInB: size, Type: "MANUAL"}
	delete(s.scp.uploads, c.params["uploadId"])
	return http.StatusNoContent, nil
}

// partETag is an invented ETag, clients must keep it verbatim with its quotes.
func partETag(number int) string {
	return fmt.Sprintf(`"etag-%d"`, number)
}

// serveS3 answers the PUT of a part to its presigned URL. It stands for S3, which
// the specification does not describe. A presigned URL takes no API token, and
// S3 rejects chunked parts.
func (s *Server) serveS3(w http.ResponseWriter, r *http.Request, size int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Header.Get("Authorization") != "" {
		s.t.Errorf("presigned URL got an Authorization header")
	}
	if r.ContentLength < 0 || len(r.TransferEncoding) > 0 {
		s.t.Errorf("part without Content-Length: %v", r.TransferEncoding)
	}
	if s.scp.partFailures > 0 {
		s.scp.partFailures--
		if s.scp.partStatus == 0 {
			conn, _, err := http.NewResponseController(w).Hijack()
			if err != nil {
				s.t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		w.WriteHeader(s.scp.partStatus)
		_, _ = w.Write([]byte("<Error><Code>MockCode</Code><Message>signature=mock</Message></Error>"))
		return
	}
	id, number, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, s3Prefix), "/")
	upload, ok := s.scp.uploads[id]
	n, err := strconv.Atoi(number)
	if r.Method != http.MethodPut || !ok || err != nil {
		s.t.Errorf("unexpected S3 request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	upload.sizes[n] = int64(size)
	w.Header().Set("ETag", partETag(n))
}

// FailParts makes the next uploads of parts answer with status, or drop the
// connection with status 0.
func (s *Server) FailParts(status, times int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.partStatus, s.scp.partFailures = status, times
}

// DelayCompletion makes the completion of an upload take d.
func (s *Server) DelayCompletion(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.completeDelay = d
}

// Object returns the size of an object, or false without one.
func (s *Server) Object(kind, key string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.scp.objects[kind][key]
	if !ok {
		return 0, false
	}
	return o.SizeInB, true
}

// RemoveObject deletes an object outside of Terraform.
func (s *Server) RemoveObject(kind, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.scp.objects[kind], key)
}
