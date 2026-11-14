package telegram

import "io"

// maxBodyBytes caps a Bot API response body we are willing to read.
const maxBodyBytes = 1 << 20

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxBodyBytes))
}

func truncate(s string, limit int) string {
	if len([]rune(s)) <= limit {
		return s
	}

	return string([]rune(s)[:limit]) + "…"
}
