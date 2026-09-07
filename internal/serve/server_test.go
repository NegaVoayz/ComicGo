package serve

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeProv struct{}

func (fakeProv) Cover(id int64) ([]byte, string, error) {
	if id < 0 {
		return nil, "", errors.New("missing")
	}
	return []byte("cover-bytes"), "image/jpeg", nil
}
func (fakeProv) Page(id int64, n int) ([]byte, string, error) {
	if n < 0 || n > 3 {
		return nil, "", errors.New("out of range")
	}
	return []byte(fmt.Sprintf("page-%d", n)), "image/png", nil
}

func TestServerEndpoints(t *testing.T) {
	s, err := Start(fakeProv{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	get := func(url string) (int, string) {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	// cover ok
	code, body := get(s.CoverURL(7))
	if code != 200 || body != "cover-bytes" {
		t.Errorf("cover: %d %q", code, body)
	}
	// page ok
	code, body = get(s.PageURL(7, 2))
	if code != 200 || body != "page-2" {
		t.Errorf("page: %d %q", code, body)
	}
	// page out of range -> 404
	code, _ = get(s.PageURL(7, 99))
	if code != http.StatusNotFound {
		t.Errorf("bad page status = %d", code)
	}
	// missing token -> 403
	resp, err := http.Get(strings.Replace(s.CoverURL(7), "?t="+s.Token, "", 1))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("no-token status = %d", resp.StatusCode)
	}
}
