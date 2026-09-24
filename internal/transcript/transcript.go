package transcript

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
)

const ChunkSize = 64 * 1024

var ErrNoUsage = errors.New("no assistant usage in transcript")

func ReadFile(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return Read(f, info.Size())
}

func Read(r io.ReaderAt, size int64) (int64, error) {
	pos := size
	buf := make([]byte, 0)
	start := 0
	for pos > 0 {
		n := int64(ChunkSize)
		if n > pos {
			n = pos
		}
		pos -= n
		buf, start = prepend(buf, start, int(n))
		if _, err := r.ReadAt(buf[start:start+int(n)], pos); err != nil && err != io.EOF {
			return 0, err
		}
		data := buf[start:]
		end := len(data)
		for {
			i := bytes.LastIndexByte(data[:end], '\n')
			if i < 0 {
				break
			}
			if tokens, ok := usage(data[i+1 : end]); ok {
				return tokens, nil
			}
			end = i
		}
		buf = buf[:start+end]
	}
	if tokens, ok := usage(buf[start:]); ok {
		return tokens, nil
	}
	return 0, ErrNoUsage
}

func prepend(buf []byte, start, n int) ([]byte, int) {
	if start >= n {
		return buf, start - n
	}
	used := len(buf) - start
	grown := make([]byte, max(2*(used+n), ChunkSize))
	newStart := len(grown) - used
	copy(grown[newStart:], buf[start:])
	return grown, newStart - n
}

type line struct {
	Type    string `json:"type"`
	Message struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

var assistantType = []byte(`"assistant"`)

func usage(raw []byte) (int64, bool) {
	if !bytes.Contains(raw, assistantType) {
		return 0, false
	}
	var l line
	if json.Unmarshal(raw, &l) != nil {
		return 0, false
	}
	if l.Type != "assistant" || l.Message.Usage == nil || l.Message.Model == "<synthetic>" {
		return 0, false
	}
	u := l.Message.Usage
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens, true
}
