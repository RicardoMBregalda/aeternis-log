package handlers

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// logCursor is the opaque keyset pagination cursor: the (created_at, id) of the
// last returned item. Pages are ordered by created_at desc, id desc. created_at
// is stored at millisecond precision to match how it is persisted in MongoDB.
type logCursor struct {
	TimeMillis int64  `json:"t"`
	ID         string `json:"id"`
}

// encodeCursor builds an opaque cursor string from a log's created_at and id.
func encodeCursor(createdAt time.Time, id string) string {
	b, _ := json.Marshal(logCursor{TimeMillis: createdAt.UnixMilli(), ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCursor parses an opaque cursor string produced by encodeCursor.
func decodeCursor(s string) (logCursor, error) {
	var cur logCursor
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cur, err
	}
	err = json.Unmarshal(raw, &cur)
	return cur, err
}

// cloneFilter returns a shallow copy of a bson.M filter so cursor predicates can
// be added without mutating the base filter used for the total count.
func cloneFilter(m bson.M) bson.M {
	out := make(bson.M, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}
