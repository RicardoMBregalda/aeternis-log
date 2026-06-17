package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// FlexTime is a time type that tolerates the several timestamp encodings found
// across MongoDB documents (BSON DateTime, RFC3339, and a few looser string
// forms), while always serializing back as RFC3339.
type FlexTime struct {
	time.Time
}

// UnmarshalBSONValue implements bsoncodec.ValueUnmarshaler for flexible time parsing.
func (ft *FlexTime) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	// Handle BSON DateTime
	if t == bsontype.DateTime {
		// BSON DateTime is 8 bytes (int64 milliseconds since epoch)
		if len(data) != 8 {
			return fmt.Errorf("invalid BSON DateTime length: %d", len(data))
		}
		ms := int64(data[0]) | int64(data[1])<<8 | int64(data[2])<<16 | int64(data[3])<<24 |
			int64(data[4])<<32 | int64(data[5])<<40 | int64(data[6])<<48 | int64(data[7])<<56
		ft.Time = time.Unix(ms/1000, (ms%1000)*1000000).UTC()
		return nil
	}

	// Handle string
	if t == bsontype.String {
		// BSON strings are: 4 bytes length + string bytes + 1 null byte
		if len(data) < 5 {
			return fmt.Errorf("invalid BSON string length: %d", len(data))
		}
		strLen := int(data[0]) | int(data[1])<<8 | int(data[2])<<16 | int(data[3])<<24
		str := string(data[4 : 4+strLen-1]) // -1 to exclude null terminator

		// Try formats in order of likelihood
		formats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.999999", // Without timezone
			"2006-01-02T15:04:05",        // Without milliseconds
			"2006-01-02 15:04:05.999999", // Space separator
			"2006-01-02 15:04:05",        // Space separator without ms
		}

		for _, format := range formats {
			if parsedTime, err := time.Parse(format, str); err == nil {
				// If parsed without timezone, assume UTC
				if parsedTime.Location() == time.UTC {
					ft.Time = parsedTime.UTC()
				} else {
					ft.Time = parsedTime
				}
				return nil
			}
		}

		return fmt.Errorf("unable to parse time string: %s", str)
	}

	return fmt.Errorf("unsupported BSON type %v for FlexTime", t)
}

// MarshalBSONValue implements bsoncodec.ValueMarshaler.
func (ft FlexTime) MarshalBSONValue() (bsontype.Type, []byte, error) {
	// Convert time to BSON DateTime (milliseconds since epoch)
	dt := primitive.NewDateTimeFromTime(ft.Time)

	// Encode the DateTime as raw bytes (8 bytes, int64)
	ms := int64(dt)
	data := make([]byte, 8)
	data[0] = byte(ms)
	data[1] = byte(ms >> 8)
	data[2] = byte(ms >> 16)
	data[3] = byte(ms >> 24)
	data[4] = byte(ms >> 32)
	data[5] = byte(ms >> 40)
	data[6] = byte(ms >> 48)
	data[7] = byte(ms >> 56)

	return bsontype.DateTime, data, nil
}

// MarshalBSON implements custom BSON marshaling for document-level use.
func (ft FlexTime) MarshalBSON() ([]byte, error) {
	return bson.Marshal(bson.M{"$date": primitive.NewDateTimeFromTime(ft.Time)})
}

// MarshalJSON renders the time as RFC3339.
func (ft FlexTime) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, ft.Time.Format(time.RFC3339))), nil
}
