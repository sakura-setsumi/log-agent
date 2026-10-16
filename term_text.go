package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/pkg/sftp"
	"golang.org/x/text/encoding/simplifiedchinese"
)

const termTextLimit = 2 << 20

var termTextSaveMu sync.Mutex

type termTextDocument struct {
	Path      string `json:"path"`
	Text      string `json:"text"`
	Encoding  string `json:"encoding"`
	ETag      string `json:"etag"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	ReadOnly  bool   `json:"readOnly"`
}

func termDecodeText(data []byte, truncated bool) (string, string, error) {
	encoding := "UTF-8"
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		var order binary.ByteOrder = binary.LittleEndian
		encoding = "UTF-16 LE"
		if data[0] == 0xfe {
			order = binary.BigEndian
			encoding = "UTF-16 BE"
		}
		data = data[2:]
		if len(data)%2 != 0 {
			if !truncated {
				return "", "", errors.New("invalid UTF-16 text")
			}
			data = data[:len(data)-1]
		}
		units := make([]uint16, len(data)/2)
		for i := range units {
			units[i] = order.Uint16(data[i*2:])
		}
		if truncated && len(units) > 0 && units[len(units)-1] >= 0xd800 && units[len(units)-1] <= 0xdbff {
			units = units[:len(units)-1]
		}
		for i := 0; i < len(units); i++ {
			if units[i] >= 0xd800 && units[i] <= 0xdbff {
				if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
					return "", "", errors.New("invalid UTF-16 text")
				}
				i++
			} else if units[i] >= 0xdc00 && units[i] <= 0xdfff {
				return "", "", errors.New("invalid UTF-16 text")
			}
		}
		data = []byte(string(utf16.Decode(units)))
	} else {
		if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
			encoding = "UTF-8 BOM"
			data = data[3:]
		}
		if !utf8.Valid(data) {
			trimmed := false
			if truncated {
				for n := 1; n <= 3 && n <= len(data); n++ {
					if utf8.Valid(data[:len(data)-n]) {
						data = data[:len(data)-n]
						trimmed = true
						break
					}
				}
			}
			if !trimmed {
				decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
				if err != nil || bytes.Contains(decoded, []byte{0xef, 0xbf, 0xbd}) {
					return "", "", errors.New("unsupported text encoding")
				}
				data = decoded
				encoding = "GB18030"
			}
		}
	}
	for _, b := range data {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' && b != '\f' {
			return "", "", errors.New("binary file")
		}
	}
	return string(data), encoding, nil
}
func termEncodeText(text, encoding string) ([]byte, error) {
	switch encoding {
	case "UTF-8":
		return []byte(text), nil
	case "UTF-8 BOM":
		return append([]byte{0xef, 0xbb, 0xbf}, []byte(text)...), nil
	case "GB18030":
		return simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(text))
	case "UTF-16 LE", "UTF-16 BE":
		var order binary.ByteOrder = binary.LittleEndian
		prefix := []byte{0xff, 0xfe}
		if encoding == "UTF-16 BE" {
			order = binary.BigEndian
			prefix = []byte{0xfe, 0xff}
		}
		units := utf16.Encode([]rune(text))
		out := make([]byte, 2+len(units)*2)
		copy(out, prefix)
		for i, u := range units {
			order.PutUint16(out[2+i*2:], u)
		}
		return out, nil
	}
	return nil, errors.New("unsupported text encoding")
}
func readTermText(client *sftp.Client, target string) (termTextDocument, os.FileInfo, error) {
	file, err := client.Open(target)
	if err != nil {
		return termTextDocument{}, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return termTextDocument{}, nil, err
	}
	if !info.Mode().IsRegular() {
		return termTextDocument{}, nil, errors.New("not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, termTextLimit+1))
	if err != nil {
		return termTextDocument{}, nil, err
	}
	truncated := len(data) > termTextLimit
	if truncated {
		data = data[:termTextLimit]
	}
	text, encoding, err := termDecodeText(data, truncated)
	if err != nil {
		return termTextDocument{}, nil, err
	}
	digest := sha256.Sum256(data)
	return termTextDocument{Path: target, Text: text, Encoding: encoding, ETag: hex.EncodeToString(digest[:]), Size: info.Size(), Truncated: truncated, ReadOnly: truncated}, info, nil
}
func handleTermText(w http.ResponseWriter, r *http.Request, session *termSession) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	target, ok := termPath(r.URL.Query().Get("path"))
	if !ok || target == "/" {
		writeJSON(w, 400, map[string]string{"error": "invalid_path"})
		return
	}
	client, err := session.files()
	if err != nil {
		termFSError(w, err)
		return
	}
	canonical, err := client.RealPath(target)
	if err != nil {
		termFSError(w, err)
		return
	}
	target = canonical
	link, err := client.Lstat(target)
	if err != nil {
		termFSError(w, err)
		return
	}
	if r.Method == http.MethodGet {
		doc, _, err := readTermText(client, target)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
				termFSError(w, err)
			} else {
				writeJSON(w, 415, map[string]string{"error": "text_not_supported", "detail": err.Error()})
			}
			return
		}
		doc.ReadOnly = doc.ReadOnly || link.Mode()&os.ModeSymlink != 0
		writeJSON(w, 200, doc)
		return
	}
	var req struct {
		Text     string `json:"text"`
		Encoding string `json:"encoding"`
		ETag     string `json:"etag"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, termTextLimit*6+1024)).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	termTextSaveMu.Lock()
	defer termTextSaveMu.Unlock()
	old, info, err := readTermText(client, target)
	if err != nil {
		termFSError(w, err)
		return
	}
	if old.ReadOnly || link.Mode()&os.ModeSymlink != 0 {
		writeJSON(w, 400, map[string]string{"error": "text_read_only"})
		return
	}
	if req.ETag == "" || old.ETag != req.ETag {
		writeJSON(w, 409, map[string]string{"error": "text_conflict", "detail": "The remote file changed. Reopen it before saving."})
		return
	}
	if req.Encoding != old.Encoding {
		writeJSON(w, 400, map[string]string{"error": "encoding_mismatch"})
		return
	}
	data, err := termEncodeText(req.Text, req.Encoding)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "encoding_failed", "detail": err.Error()})
		return
	}
	if len(data) > termTextLimit {
		writeJSON(w, 413, map[string]string{"error": "text_too_large"})
		return
	}
	if _, ok := client.HasExtension("posix-rename@openssh.com"); !ok {
		writeJSON(w, 400, map[string]string{"error": "atomic_save_unavailable", "detail": "This SFTP server does not support atomic replacement."})
		return
	}
	temporary := pathpkg.Join(pathpkg.Dir(target), ".log-agent-edit-"+newTermID())
	defer client.Remove(temporary)
	file, err := client.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		termFSError(w, err)
		return
	}
	_, err = file.Write(data)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = client.Chmod(temporary, info.Mode().Perm())
	}
	if err == nil {
		if stat, ok := info.Sys().(*sftp.FileStat); ok {
			err = client.Chown(temporary, int(stat.UID), int(stat.GID))
		}
	}
	if err != nil {
		termFSError(w, err)
		return
	}
	current, _, err := readTermText(client, target)
	if err != nil {
		termFSError(w, err)
		return
	}
	if current.ETag != old.ETag {
		writeJSON(w, 409, map[string]string{"error": "text_conflict", "detail": "The remote file changed. Reopen it before saving."})
		return
	}
	if err = client.PosixRename(temporary, target); err != nil {
		termFSError(w, err)
		return
	}
	digest := sha256.Sum256(data)
	writeJSON(w, 200, map[string]any{"ok": true, "etag": hex.EncodeToString(digest[:]), "size": len(data)})
}
