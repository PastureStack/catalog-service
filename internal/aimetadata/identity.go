package aimetadata

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

// The cache primary key is deliberately absent from this logical identity.
// Project scope uses the existing Engine-verified account namespace; global
// Catalogs use the literal global namespace. Mutable metadata/versions are not
// identity, while a source/branch/pin/template-location change is.
type identity struct {
	Scope, Catalog, Repo, Branch, Pin, Folder, Base string
}

func logicalID(value identity) (string, error) {
	u, err := url.Parse(value.Repo)
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.Opaque != "" {
		return "", errors.New("invalid catalog source identity")
	}
	u.User = nil
	u.RawQuery, u.Fragment, u.RawFragment = "", "", ""
	u.ForceQuery = false
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	if value.Scope != "global" && !projectID.MatchString(value.Scope) {
		return "", errors.New("invalid catalog scope")
	}
	encoded, err := json.Marshal([]string{"catalog-template-v1", value.Scope, value.Catalog,
		u.String(), value.Branch, strings.ToLower(value.Pin), value.Folder, value.Base})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "ctm-" + hex.EncodeToString(digest[:]), nil
}

// A cursor is a private locator hint, never an authorization target or grant.
// Its context binding prevents accidental reuse across project/query. The
// source must re-read its anchor and compare the logical identity before use.
func cursor(project, query string, row int64, id string) string {
	data := make([]byte, 57)
	data[0] = 1
	binary.BigEndian.PutUint64(data[1:9], uint64(row))
	digest, _ := hex.DecodeString(strings.TrimPrefix(id, "ctm-"))
	copy(data[9:41], digest)
	bound := sha256.Sum256([]byte(project + "\x00" + query))
	copy(data[41:], bound[:16])
	return "cmc-" + base64.RawURLEncoding.EncodeToString(data)
}

func parseCursor(value, project, query string) (int64, string, bool) {
	if !strings.HasPrefix(value, "cmc-") {
		return 0, "", false
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "cmc-"))
	if err != nil || len(data) != 57 || data[0] != 1 {
		return 0, "", false
	}
	row := binary.BigEndian.Uint64(data[1:9])
	if row == 0 || row > uint64(^uint64(0)>>1) {
		return 0, "", false
	}
	bound := sha256.Sum256([]byte(project + "\x00" + query))
	if subtle.ConstantTimeCompare(data[41:], bound[:16]) != 1 {
		return 0, "", false
	}
	return int64(row), "ctm-" + hex.EncodeToString(data[9:41]), true
}
