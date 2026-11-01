// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Disk mirrors the Disk schema.
type Disk struct {
	Name            string `json:"name"`
	AllocationInMiB *int64 `json:"allocationInMiB"`
	CapacityInMiB   *int64 `json:"capacityInMiB"`
	StorageDriver   string `json:"storageDriver"`
}

// S3DownloadInfos mirrors the S3DownloadInfos schema.
type S3DownloadInfos struct {
	Filename                            string              `json:"filename"`
	PresignedURL                        string              `json:"presignedUrl"`
	PresignedURLValidityDurationInHours *int64              `json:"presignedUrlValidityDurationInHours"`
	Headers                             map[string][]string `json:"headers"`
}

// GuestAgentData mirrors the GuestAgentData schema. Data is JSON whose content
// changes with the agent version, and nil if netcup sends none.
type GuestAgentData struct {
	GuestAgentAvailable *bool            `json:"guestAgentAvailable"`
	GuestAgentData      *json.RawMessage `json:"guestAgentData"`
}

// ImageMinimal mirrors the ImageMinimal schema.
type ImageMinimal struct {
	ID   *int64 `json:"id"`
	Name string `json:"name"`
}

// ImageFlavour mirrors the ImageFlavour schema.
type ImageFlavour struct {
	ID    *int64        `json:"id"`
	Name  string        `json:"name"`
	Alias string        `json:"alias"`
	Text  string        `json:"text"`
	Image *ImageMinimal `json:"image"`
}

// IsoImage mirrors the IsoImage schema.
type IsoImage struct {
	ID           *int64  `json:"id"`
	Name         string  `json:"name"`
	Filename     string  `json:"filename"`
	Description  *string `json:"description"`
	Architecture string  `json:"architecture"`
}

// UserMinimal mirrors the UserMinimal schema.
type UserMinimal struct {
	ID        *int64 `json:"id"`
	Username  string `json:"username"`
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	Email     string `json:"email"`
	Company   string `json:"company"`
}

// Log mirrors the Log schema.
type Log struct {
	Type          string       `json:"type"`
	ExecutingUser *UserMinimal `json:"executingUser"`
	Date          string       `json:"date"`
	LogKey        string       `json:"logKey"`
	Message       string       `json:"message"`
}

// TaskProgress mirrors the TaskProgress schema.
type TaskProgress struct {
	ExpectedFinishedAt string   `json:"expectedFinishedAt"`
	ProgressInPercent  *float64 `json:"progressInPercent"`
}

// TaskInfoMinimal mirrors the TaskInfoMinimal schema.
type TaskInfoMinimal struct {
	UUID          string        `json:"uuid"`
	Name          string        `json:"name"`
	State         string        `json:"state"`
	StartedAt     string        `json:"startedAt"`
	FinishedAt    string        `json:"finishedAt"`
	ExecutingUser *UserMinimal  `json:"executingUser"`
	TaskProgress  *TaskProgress `json:"taskProgress"`
	Message       string        `json:"message"`
	OnRollback    *bool         `json:"onRollback"`
}

// SnapshotMinimal mirrors the SnapshotMinimal schema.
type SnapshotMinimal struct {
	UUID              string   `json:"uuid"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Disks             []string `json:"disks"`
	CreationTime      string   `json:"creationTime"`
	State             string   `json:"state"`
	Online            *bool    `json:"online"`
	Exported          *bool    `json:"exported"`
	ExportedSizeInKiB *int64   `json:"exportedSizeInKiB"`
}

// Page selects a part of a list. A zero Limit reads all entries from Offset on.
type Page struct {
	Limit  int
	Offset int
}

// listPage reads the entries of a list that page selects.
func listPage[T any](ctx context.Context, c *Client, path string, filter url.Values, page Page) ([]T, error) {
	if page.Limit == 0 {
		return listAll[T](ctx, c, path, filter, page.Offset)
	}
	return listOnePage[T](ctx, c, path, filter, page.Limit, page.Offset)
}

func serverPath(serverID int) string {
	return "/servers/" + strconv.Itoa(serverID)
}

// ListDisks returns the disks of a server.
func (c *Client) ListDisks(ctx context.Context, serverID int) ([]Disk, error) {
	var out []Disk
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/disks", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetDisk returns one disk of a server.
func (c *Client) GetDisk(ctx context.Context, serverID int, name string) (*Disk, error) {
	var out Disk
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/disks/"+url.PathEscape(name), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListSupportedDrivers returns the storage drivers a server supports.
func (c *Client) ListSupportedDrivers(ctx context.Context, serverID int) ([]string, error) {
	var out []string
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/disks/supported-drivers", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetGPUDriver returns the download of the GPU driver of a server.
func (c *Client) GetGPUDriver(ctx context.Context, serverID int) (*S3DownloadInfos, error) {
	var out S3DownloadInfos
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/gpu-driver", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetGuestAgent returns the data of the guest agent of a server.
func (c *Client) GetGuestAgent(ctx context.Context, serverID int) (*GuestAgentData, error) {
	var out GuestAgentData
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/guest-agent", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetGuestAgentStatus returns the available value of the guest agent status.
func (c *Client) GetGuestAgentStatus(ctx context.Context, serverID int) (*bool, error) {
	var out struct {
		Available *bool `json:"available"`
	}
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/guest-agent/status", nil, &out); err != nil {
		return nil, err
	}
	return out.Available, nil
}

// ListImageFlavours returns the image flavours a server can install.
func (c *Client) ListImageFlavours(ctx context.Context, serverID int) ([]ImageFlavour, error) {
	var out []ImageFlavour
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/imageflavours", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListIsoImages returns the ISO images a server can attach.
func (c *Client) ListIsoImages(ctx context.Context, serverID int) ([]IsoImage, error) {
	var out []IsoImage
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/isoimages", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListServerLogs returns the logs of a server.
func (c *Client) ListServerLogs(ctx context.Context, serverID int, page Page) ([]Log, error) {
	return listPage[Log](ctx, c, serverPath(serverID)+"/logs", nil, page)
}

// Metric is the path of a metric below a server.
type Metric string

// The metrics of a server.
const (
	MetricCPU           Metric = "/metrics/cpu"
	MetricDisk          Metric = "/metrics/disk"
	MetricNetwork       Metric = "/metrics/network"
	MetricNetworkPacket Metric = "/metrics/network/packet"
)

// GetMetrics returns a metric as JSON, a map from an ISO-8601 timestamp to the
// values at that time. A nil hours uses the default of netcup.
func (c *Client) GetMetrics(ctx context.Context, serverID int, metric Metric, hours *int64) (json.RawMessage, error) {
	target := serverPath(serverID) + string(metric)
	if hours != nil {
		target += "?hours=" + strconv.FormatInt(*hours, 10)
	}
	var out json.RawMessage
	if err := c.do(ctx, http.MethodGet, target, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSnapshots returns the snapshots of a server.
func (c *Client) ListSnapshots(ctx context.Context, serverID int) ([]SnapshotMinimal, error) {
	var out []SnapshotMinimal
	if err := c.do(ctx, http.MethodGet, snapBase(serverID), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// snapshotDryRun is the body of POST .../snapshots:dryrun.
type snapshotDryRun struct {
	DiskName       string `json:"diskName,omitempty"`
	OnlineSnapshot bool   `json:"onlineSnapshot"`
}

// DryRunSnapshot returns the errors netcup would give for this snapshot. An
// empty list means it is possible.
func (c *Client) DryRunSnapshot(ctx context.Context, serverID int, diskName string, online bool) ([]ResponseError, error) {
	target := c.BaseURL + snapBase(serverID) + ":dryrun"
	status, raw, err := c.exchange(ctx, http.MethodPost, target, "application/json", snapshotDryRun{DiskName: diskName, OnlineSnapshot: online})
	if err != nil {
		return nil, err
	}
	var out []ResponseError
	if status == http.StatusBadRequest && json.Unmarshal(raw, &out) == nil {
		return out, nil
	}
	if err := decodeResponse(http.MethodPost, target, status, raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// TaskFilter selects tasks by the query parameters of GET /tasks. A nil field
// does not filter.
type TaskFilter struct {
	Q        *string
	ServerID *int64
	State    *string
}

// ListTasks returns the tasks matching the filter.
func (c *Client) ListTasks(ctx context.Context, filter TaskFilter, page Page) ([]TaskInfoMinimal, error) {
	values := url.Values{}
	if filter.Q != nil {
		values.Set("q", *filter.Q)
	}
	if filter.ServerID != nil {
		values.Set("serverId", strconv.FormatInt(*filter.ServerID, 10))
	}
	if filter.State != nil {
		values.Set("state", *filter.State)
	}
	return listPage[TaskInfoMinimal](ctx, c, "/tasks", values, page)
}

// Iso mirrors the Iso schema.
type Iso struct {
	IsoAttached bool   `json:"isoAttached"`
	Iso         string `json:"iso"`
}

// serverAttachIso mirrors the ServerAttachIso schema.
type serverAttachIso struct {
	IsoID                   *int64  `json:"isoId,omitempty"`
	UserIsoName             *string `json:"userIsoName,omitempty"`
	ChangeBootDeviceToCdrom *bool   `json:"changeBootDeviceToCdrom,omitempty"`
}

// GetIso returns the ISO attached to a server.
func (c *Client) GetIso(ctx context.Context, serverID int) (*Iso, error) {
	var out Iso
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/iso", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AttachIso attaches an ISO and waits for the task. Exactly one of isoID and
// userIsoName names the ISO. started tells whether netcup accepted the request,
// also if the task fails afterwards.
func (c *Client) AttachIso(ctx context.Context, serverID int, isoID *int64, userIsoName *string, changeBootDeviceToCdrom *bool) (started bool, err error) {
	var task TaskInfo
	body := serverAttachIso{IsoID: isoID, UserIsoName: userIsoName, ChangeBootDeviceToCdrom: changeBootDeviceToCdrom}
	if err := c.do(ctx, http.MethodPost, serverPath(serverID)+"/iso", body, &task); err != nil {
		return false, err
	}
	return true, c.waitForTask(ctx, task.UUID)
}

// DetachIso detaches the ISO of a server.
func (c *Client) DetachIso(ctx context.Context, serverID int) error {
	return c.do(ctx, http.MethodDelete, serverPath(serverID)+"/iso", nil, nil)
}

// RescueSystemNotActive is the code of netcup for deactivating an inactive rescue
// system.
const RescueSystemNotActive = "server.rescuesystem.notactive"

// RescueSystemStatus mirrors the RescueSystemStatus schema.
type RescueSystemStatus struct {
	Active   *bool  `json:"active"`
	Password string `json:"password"`
}

// GetRescueSystem returns the status of the rescue system of a server.
func (c *Client) GetRescueSystem(ctx context.Context, serverID int) (*RescueSystemStatus, error) {
	var out RescueSystemStatus
	if err := c.do(ctx, http.MethodGet, serverPath(serverID)+"/rescuesystem", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ActivateRescueSystem activates the rescue system of a shut down server and
// waits for the task.
// started tells whether netcup accepted the request, also if the task fails
// afterwards.
func (c *Client) ActivateRescueSystem(ctx context.Context, serverID int) (started bool, err error) {
	var task TaskInfo
	if err := c.do(ctx, http.MethodPost, serverPath(serverID)+"/rescuesystem", nil, &task); err != nil {
		return false, err
	}
	return true, c.waitForTask(ctx, task.UUID)
}

// DeactivateRescueSystem deactivates the rescue system, which shuts the server
// down, and waits for the task.
func (c *Client) DeactivateRescueSystem(ctx context.Context, serverID int) error {
	var task TaskInfo
	if err := c.do(ctx, http.MethodDelete, serverPath(serverID)+"/rescuesystem", nil, &task); err != nil {
		return err
	}
	return c.waitForTask(ctx, task.UUID)
}
