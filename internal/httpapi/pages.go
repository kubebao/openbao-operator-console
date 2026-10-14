package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

const activityList = "activity"

// A cursor identifies a position, not an access grant. Every page resolves current
// authorization again. List binds the cursor to its endpoint, deployment, and window.
type pageCursor struct {
	List string    `json:"list"`
	At   time.Time `json:"at"`
	ID   string    `json:"id"`
}

func historyList(id, since string) string { return "history:" + id + ":" + since }

func encodeCursor(list string, position *fleet.Cursor) (*string, error) {
	if position == nil {
		return nil, nil
	}
	// Strings and time.Time always encode successfully for valid stored timestamps.
	payload, err := json.Marshal(pageCursor{List: list, At: position.RecordedAt, ID: position.ID})
	if err != nil {
		return nil, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return &encoded, nil
}

func pageQuery(values url.Values, list string) (fleet.Page, error) {
	page := fleet.Page{Limit: 100}
	if values.Has("limit") {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit < 1 || limit > 500 || len(values["limit"]) != 1 {
			return page, badRequest("The page limit must be an integer from 1 to 500.")
		}
		page.Limit = limit
	}
	if !values.Has("cursor") {
		return page, nil
	}
	value := values.Get("cursor")
	if value == "" || len(value) > 512 || len(values["cursor"]) != 1 {
		return page, badRequest("The page cursor is invalid. Start from the first page.")
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return page, badRequest("The page cursor is invalid. Start from the first page.")
	}
	var cursor pageCursor
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil || cursor.List != list || cursor.At.IsZero() || cursor.At.Year() < 1 ||
		cursor.ID == "" || len(cursor.ID) > 128 || decoder.Decode(new(json.RawMessage)) != io.EOF {
		return page, badRequest("The page cursor is invalid. Start from the first page.")
	}
	page.After = &fleet.Cursor{RecordedAt: cursor.At, ID: cursor.ID}
	return page, nil
}

func historyQuery(values url.Values, list string) (fleet.HistoryQuery, error) {
	page, err := pageQuery(values, list)
	query := fleet.HistoryQuery{Page: page}
	if err != nil {
		return query, err
	}
	if page.After != nil {
		id, err := strconv.ParseInt(page.After.ID, 10, 64)
		if err != nil || id < 1 {
			return query, badRequest("The history cursor is invalid. Start from the first page.")
		}
	}
	if values.Has("since") {
		since, err := time.Parse(time.RFC3339Nano, values.Get("since"))
		if err != nil || since.IsZero() || since.Year() < 1 || len(values["since"]) != 1 {
			return query, badRequest("The history start must be an RFC 3339 timestamp.")
		}
		query.Since = &since
	}
	return query, nil
}

func (s *Server) readActivity(w http.ResponseWriter, r *http.Request, user config.User) error {
	query, err := pageQuery(r.URL.Query(), activityList)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	page, err := s.Fleet.Activity(ctx, user.Scopes, query)
	if err != nil {
		return unavailable(err, "Recorded activity is temporarily unavailable.")
	}
	nextCursor, err := encodeCursor(activityList, page.Next)
	if err != nil {
		return unavailable(err, "Recorded activity is temporarily unavailable.")
	}
	jsonResponse(w, http.StatusOK, contract.ActivityPage{
		Events:     mapSlice(page.Events, activityContract),
		NextCursor: nextCursor,
	})
	return nil
}
