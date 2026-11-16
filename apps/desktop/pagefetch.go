package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// The app calls the server's API from the web admin's page, with the
// admin's own session cookie, so that it works however the user signed in:
// a password sign-in leaves no token in the Keychain. The web view keeps
// running while the main window is hidden (BackgroundThrottling off).

// pageFetchTimeout bounds a request, in the page.
const pageFetchTimeout = 20 * time.Second

// pageResult is what the page's fetch answered; Status 0 for a network
// error, which Error tells.
type pageResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Error  string `json:"error"`
}

// err is the result as an error: nil for a 2xx, an *apiError for another
// status, with the message of Mix Space's error envelope.
func (r pageResult) err() error {
	if r.Status == 0 {
		if r.Error == "" {
			r.Error = "no answer"
		}
		return errors.New(r.Error)
	}
	if r.Status >= 200 && r.Status <= 299 {
		return nil
	}
	e := &apiError{Status: r.Status}
	var env struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if json.Unmarshal([]byte(r.Body), &env) == nil {
		e.Code, e.Message = env.Error.Code, env.Error.Message
	}
	return e
}

// data decodes the success envelope's data into out.
func (r pageResult) data(out any) error {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(r.Body), &env); err != nil {
		return err
	}
	if len(env.Data) == 0 {
		return errors.New("the server sent no data")
	}
	return json.Unmarshal(env.Data, out)
}

// pageFetchScript is the page's request: body is sent as JSON, nil for none.
func pageFetchScript(method, url string, headers map[string]string, body any) (string, error) {
	h := map[string]string{"accept": "application/json"}
	for k, v := range headers {
		h[k] = v
	}
	init := map[string]any{"method": method, "credentials": "include", "headers": h}
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		h["content-type"] = "application/json"
		init["body"] = string(data)
	}
	u, _ := json.Marshal(url)
	i, _ := json.Marshal(init)
	return fmt.Sprintf(`
try {
  const r = await fetch(%s, Object.assign(%s, {signal: AbortSignal.timeout(%d)}));
  return {status: r.status, body: await r.text()};
} catch (e) {
  return {status: 0, error: String(e && e.message || e)};
}`, u, i, pageFetchTimeout.Milliseconds()), nil
}

// pageFetch sends a request to the API from the page, off the main thread.
func pageFetch(page *mygo.Page, method, url string, headers map[string]string, body any) (pageResult, error) {
	script, err := pageFetchScript(method, url, headers, body)
	if err != nil {
		return pageResult{}, err
	}
	res, err := mygo.EvalAs[pageResult](page, script)
	if err != nil {
		return res, err
	}
	return res, res.err()
}

// signedIn reports whether the web admin shows a signed-in page, from which
// the API can be called.
func (a *app) signedIn() bool { return a.web != nil && a.pageKind() == pageMain }

// apiErrorText is an error of the API as the user reads it.
func (a *app) apiErrorText(err error) string {
	var ae *apiError
	switch {
	case isUnauthorized(err):
		return a.tr("登录已过期，请重新登录", "The session expired. Sign in again")
	case errors.As(err, &ae) && ae.Message != "":
		return ae.Message
	}
	return err.Error()
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// excerpt is s cut to n characters, on one line.
func excerpt(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func appUserData() (string, error) { return mygo.App.Path(mygo.PathUserData) }
