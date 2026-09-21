package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.spotify.com/v1"

type TokenProvider interface {
	AccessToken(context.Context, bool) (string, error)
}

type Client struct {
	BaseURL string
	HTTP    *http.Client
	Tokens  TokenProvider
}

func New(tokens TokenProvider) *Client {
	return &Client{BaseURL: defaultBaseURL, HTTP: http.DefaultClient, Tokens: tokens}
}

type APIError struct {
	Status     int
	Message    string
	Reason     string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized:
		return "Spotify login expired; run `sonicli auth login`"
	case http.StatusForbidden:
		msg := strings.ToLower(e.Message + " " + e.Reason)
		if strings.Contains(msg, "premium") {
			return "this playback action requires Spotify Premium"
		}
		if strings.Contains(msg, "device") || strings.Contains(msg, "player") {
			return "no usable Spotify device; open Spotify on a device or choose one with `d`"
		}
		return "Spotify denied this action; check the account, app allowlist, and requested permissions"
	case http.StatusNotFound:
		msg := strings.ToLower(e.Message + " " + e.Reason)
		if strings.Contains(msg, "active device") || strings.Contains(msg, "no device") {
			return "no active Spotify device; run `sonicli player status`, pair librespot, or open Spotify on another device"
		}
		if e.Message != "" {
			return fmt.Sprintf("Spotify API (404): %s", e.Message)
		}
		return "Spotify could not find the requested item"
	case http.StatusTooManyRequests:
		if e.RetryAfter > 0 {
			return fmt.Sprintf("Spotify rate limit reached; retry in %s", e.RetryAfter.Round(time.Second))
		}
		return "Spotify rate limit reached; retry later"
	default:
		if e.Message != "" {
			return fmt.Sprintf("Spotify API (%d): %s", e.Status, e.Message)
		}
		return fmt.Sprintf("Spotify API returned HTTP %d", e.Status)
	}
}

func (c *Client) Me(ctx context.Context) (User, error) {
	var out User
	err := c.do(ctx, http.MethodGet, "/me", nil, nil, &out)
	return out, err
}

func (c *Client) Playback(ctx context.Context) (Playback, error) {
	var out Playback
	err := c.do(ctx, http.MethodGet, "/me/player", nil, nil, &out)
	out.FetchedAt = time.Now()
	return out, err
}

func (c *Client) Queue(ctx context.Context) (Queue, error) {
	var out Queue
	err := c.do(ctx, http.MethodGet, "/me/player/queue", nil, nil, &out)
	return out, err
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var out struct {
		Devices []Device `json:"devices"`
	}
	err := c.do(ctx, http.MethodGet, "/me/player/devices", nil, nil, &out)
	return out.Devices, err
}

func (c *Client) Search(ctx context.Context, query string) (SearchResults, error) {
	var out SearchResults
	params := url.Values{"q": {query}, "type": {"track,album,artist,playlist"}, "limit": {"10"}}
	err := c.do(ctx, http.MethodGet, "/search", params, nil, &out)
	return out, err
}

func (c *Client) SavedTracks(ctx context.Context, limit, offset int) (Page[SavedTrack], error) {
	var out Page[SavedTrack]
	err := c.do(ctx, http.MethodGet, "/me/tracks", paging(limit, offset), nil, &out)
	return out, err
}

func (c *Client) SavedAlbums(ctx context.Context, limit, offset int) (Page[SavedAlbum], error) {
	var out Page[SavedAlbum]
	err := c.do(ctx, http.MethodGet, "/me/albums", paging(limit, offset), nil, &out)
	return out, err
}

func (c *Client) Playlists(ctx context.Context, limit, offset int) (Page[Playlist], error) {
	var out Page[Playlist]
	err := c.do(ctx, http.MethodGet, "/me/playlists", paging(limit, offset), nil, &out)
	return out, err
}

func (c *Client) PlaylistItems(ctx context.Context, id string, limit, offset int) (Page[PlaylistItem], error) {
	var out Page[PlaylistItem]
	err := c.do(ctx, http.MethodGet, "/playlists/"+url.PathEscape(id)+"/items", paging(limit, offset), nil, &out)
	return out, err
}

func (c *Client) Play(ctx context.Context, uris []string, contextURI, deviceID string) error {
	body := map[string]any{}
	if contextURI != "" {
		body["context_uri"] = contextURI
	} else if len(uris) > 0 {
		body["uris"] = uris
	}
	params := deviceParam(deviceID)
	return c.do(ctx, http.MethodPut, "/me/player/play", params, body, nil)
}

func (c *Client) Pause(ctx context.Context, deviceID string) error {
	return c.do(ctx, http.MethodPut, "/me/player/pause", deviceParam(deviceID), nil, nil)
}

func (c *Client) Next(ctx context.Context, deviceID string) error {
	return c.do(ctx, http.MethodPost, "/me/player/next", deviceParam(deviceID), nil, nil)
}

func (c *Client) Previous(ctx context.Context, deviceID string) error {
	return c.do(ctx, http.MethodPost, "/me/player/previous", deviceParam(deviceID), nil, nil)
}

func (c *Client) Seek(ctx context.Context, position int, deviceID string) error {
	params := deviceParam(deviceID)
	params.Set("position_ms", strconv.Itoa(max(0, position)))
	return c.do(ctx, http.MethodPut, "/me/player/seek", params, nil, nil)
}

func (c *Client) Volume(ctx context.Context, volume int, deviceID string) error {
	params := deviceParam(deviceID)
	params.Set("volume_percent", strconv.Itoa(min(100, max(0, volume))))
	return c.do(ctx, http.MethodPut, "/me/player/volume", params, nil, nil)
}

func (c *Client) Shuffle(ctx context.Context, enabled bool, deviceID string) error {
	params := deviceParam(deviceID)
	params.Set("state", strconv.FormatBool(enabled))
	return c.do(ctx, http.MethodPut, "/me/player/shuffle", params, nil, nil)
}

func (c *Client) Repeat(ctx context.Context, state, deviceID string) error {
	params := deviceParam(deviceID)
	params.Set("state", state)
	return c.do(ctx, http.MethodPut, "/me/player/repeat", params, nil, nil)
}

func (c *Client) AddToQueue(ctx context.Context, uri, deviceID string) error {
	params := deviceParam(deviceID)
	params.Set("uri", uri)
	return c.do(ctx, http.MethodPost, "/me/player/queue", params, nil, nil)
}

func (c *Client) Transfer(ctx context.Context, deviceID string, play bool) error {
	return c.do(ctx, http.MethodPut, "/me/player", nil, map[string]any{"device_ids": []string{deviceID}, "play": play}, nil)
}

func (c *Client) SaveLibrary(ctx context.Context, uris []string) error {
	params := url.Values{"uris": {strings.Join(uris, ",")}}
	return c.do(ctx, http.MethodPut, "/me/library", params, nil, nil)
}

func (c *Client) RemoveLibrary(ctx context.Context, uris []string) error {
	params := url.Values{"uris": {strings.Join(uris, ",")}}
	return c.do(ctx, http.MethodDelete, "/me/library", params, nil, nil)
}

func (c *Client) ContainsLibrary(ctx context.Context, uris []string) ([]bool, error) {
	var out []bool
	params := url.Values{"uris": {strings.Join(uris, ",")}}
	err := c.do(ctx, http.MethodGet, "/me/library/contains", params, nil, &out)
	return out, err
}

func paging(limit, offset int) url.Values {
	if limit <= 0 || limit > 50 {
		limit = 25
	}
	return url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(max(0, offset))}}
}

func deviceParam(id string) url.Values {
	params := url.Values{}
	if id != "" {
		params.Set("device_id", id)
	}
	return params
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	if c.Tokens == nil {
		return errors.New("Spotify token provider is not configured")
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.Tokens.AccessToken(ctx, attempt == 1)
		if err != nil {
			return err
		}
		var payload []byte
		if body != nil {
			payload, err = json.Marshal(body)
			if err != nil {
				return err
			}
		}
		reqURL := strings.TrimRight(c.BaseURL, "/") + path
		if len(query) > 0 {
			reqURL += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return fmt.Errorf("Spotify is unreachable: %w", err)
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return parseAPIError(resp, responseBody)
		}
		if out == nil || resp.StatusCode == http.StatusNoContent || len(responseBody) == 0 {
			return nil
		}
		if err := json.Unmarshal(responseBody, out); err != nil {
			return fmt.Errorf("decode Spotify response: %w", err)
		}
		return nil
	}
	return errors.New("Spotify login expired; run `sonicli auth login`")
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func parseAPIError(resp *http.Response, body []byte) error {
	apiErr := &APIError{Status: resp.StatusCode}
	var envelope struct {
		Error struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		apiErr.Message = envelope.Error.Message
		apiErr.Reason = envelope.Error.Reason
	}
	if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
		apiErr.RetryAfter = time.Duration(seconds) * time.Second
	}
	return apiErr
}
