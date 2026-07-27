package cloud189

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/httpx"
)

const (
	webURL    = "https://cloud.189.cn"
	authURL   = "https://open.e.189.cn"
	apiURL    = "https://api.cloud.189.cn"
	uploadURL = "https://upload.cloud.189.cn"

	appID      = "8025431004"
	clientType = "10020"
	version    = "6.2"
	pcType     = "TELEPC"
	channelID  = "web_cloud.189.cn"
	returnURL  = "https://m.cloud.189.cn/zhuanti/2020/loginErrorPc/index.html"

	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

	listPageSize            = 1000
	defaultOperationDelayMS = 500
	downloadURLTTLSeconds   = 300
	defaultUploadPartSize   = 10 * 1024 * 1024
	qrCodeTimeoutSec        = 300
)

func (d *Driver) rootID() string {
	if id := strings.TrimSpace(d.add.RootFolderID); id != "" && id != "/" && id != "0" {
		return id
	}
	return "-11"
}

func (d *Driver) normalizeParent(parentID string) string {
	p := strings.TrimSpace(parentID)
	if p == "" || p == "/" || p == "0" || p == "root" {
		return d.rootID()
	}
	return p
}

func (d *Driver) hasSession() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sessionKey != "" && d.sessionSecret != ""
}

func (d *Driver) currentSession() (string, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sessionKey, d.sessionSecret
}

func (d *Driver) waitInterval(ctx context.Context) error {
	return driver.WaitRequestInterval(ctx, d.intervalGate, defaultOperationDelayMS)
}

func clientSuffix() url.Values {
	return url.Values{
		"clientType": {pcType},
		"version":    {version},
		"channelId":  {channelID},
		"rand":       {fmt.Sprintf("%d_%d", rand.Intn(100000), rand.Int63n(10000000000))},
	}
}

func set189Headers(req *http.Request) {
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", webURL)
}

func newRequestID() string { return uuid.NewString() }

func itoa(v int) string { return strconv.Itoa(v) }

func (d *Driver) apiRequest(ctx context.Context, method, rawURL string, params map[string]string, out any) error {
	if !d.hasSession() {
		if _, err := d.doRefresh(ctx); err != nil {
			return err
		}
	}
	if err := d.signedJSON(ctx, method, rawURL, params, out); isSessionExpired(err) {
		if _, rerr := d.doRefresh(ctx); rerr != nil {
			return rerr
		}
		return d.signedJSON(ctx, method, rawURL, params, out)
	} else if err != nil {
		return err
	}
	return nil
}

func (d *Driver) formRequest(ctx context.Context, method, rawURL string, form url.Values, out any) error {
	if !d.hasSession() {
		if _, err := d.doRefresh(ctx); err != nil {
			return err
		}
	}
	if err := d.signedForm(ctx, method, rawURL, form, out); isSessionExpired(err) {
		if _, rerr := d.doRefresh(ctx); rerr != nil {
			return rerr
		}
		return d.signedForm(ctx, method, rawURL, form, out)
	} else if err != nil {
		return err
	}
	return nil
}

func (d *Driver) signedJSON(ctx context.Context, method, rawURL string, params map[string]string, out any) error {
	query := clientSuffix()
	for k, v := range params {
		if strings.TrimSpace(v) != "" {
			query.Set(k, v)
		}
	}
	headers, err := d.signatureHeaders(method, rawURL, "")
	if err != nil {
		return err
	}
	return d.rawJSON(ctx, method, rawURL, query, nil, headers, out)
}

func (d *Driver) signedForm(ctx context.Context, method, rawURL string, form url.Values, out any) error {
	query := clientSuffix()
	headers, err := d.signatureHeaders(method, rawURL, "")
	if err != nil {
		return err
	}
	return d.rawForm(ctx, method, rawURL, query, form, headers, out)
}

func (d *Driver) rawJSON(ctx context.Context, method, rawURL string, query url.Values, body any, extraHeaders map[string]string, out any) error {
	if err := d.waitInterval(ctx); err != nil {
		return err
	}
	req, err := httpx.NewJSONRequest(ctx, method, rawURL, query, body)
	if err != nil {
		return domain.Wrap(domain.CodeInternal, err)
	}
	set189Headers(req)
	httpx.SetHeaders(req, extraHeaders)
	resp, data, err := httpx.Execute(d.client, req, httpx.DefaultReadLimit)
	if err != nil {
		return domain.Wrap(domain.CodeDriverError, err)
	}
	if resp.StatusCode != http.StatusOK {
		return domain.Errorf(domain.CodeDriverError, "天翼云盘 API HTTP %d: %s", resp.StatusCode, httpx.Truncate(data, 300))
	}
	return parse189Response(data, out)
}

func (d *Driver) rawForm(ctx context.Context, method, rawURL string, query url.Values, form url.Values, extraHeaders map[string]string, out any) error {
	if err := d.waitInterval(ctx); err != nil {
		return err
	}
	u := rawURL
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, strings.NewReader(form.Encode()))
	if err != nil {
		return domain.Wrap(domain.CodeInternal, err)
	}
	set189Headers(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpx.SetHeaders(req, extraHeaders)
	resp, data, err := httpx.Execute(d.client, req, httpx.DefaultReadLimit)
	if err != nil {
		return domain.Wrap(domain.CodeDriverError, err)
	}
	if resp.StatusCode != http.StatusOK {
		return domain.Errorf(domain.CodeDriverError, "天翼云盘 API HTTP %d: %s", resp.StatusCode, httpx.Truncate(data, 300))
	}
	return parse189Response(data, out)
}

func parse189Response(data []byte, out any) error {
	if bytes.Contains(data, []byte("InvalidSessionKey")) || bytes.Contains(data, []byte("userSessionBO is null")) {
		return domain.Errorf(domain.CodeAuthExpired, "天翼云盘会话已失效")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '<' {
		return parse189XMLResponse(trimmed, out)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(data, &env); err != nil {
		return domain.Errorf(domain.CodeDriverError, "天翼云盘 API 返回非 JSON: %s", httpx.Truncate(data, 300))
	}
	if raw := env["res_code"]; len(raw) > 0 && !successResCode(raw) {
		return domain.Errorf(domain.CodeDriverError, "天翼云盘 API 错误：%s", responseMessage(env, "res_message", "message", "msg"))
	}
	if raw := env["code"]; len(raw) > 0 {
		var code string
		_ = json.Unmarshal(raw, &code)
		if code != "" && code != "SUCCESS" {
			return domain.Errorf(domain.CodeDriverError, "天翼云盘 API 错误(%s)：%s", code, responseMessage(env, "message", "msg"))
		}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return domain.Wrap(domain.CodeDriverError, err)
	}
	return nil
}

func parse189XMLResponse(data []byte, out any) error {
	var env struct {
		XMLName      xml.Name
		Code         string `xml:"code"`
		Message      string `xml:"message"`
		ErrorCode    string `xml:"errorCode"`
		ErrorMessage string `xml:"errorMessage"`
	}
	if err := xml.Unmarshal(data, &env); err != nil {
		return domain.Errorf(domain.CodeDriverError, "天翼云盘 API 返回无法解析: %s", httpx.Truncate(data, 300))
	}
	code := firstString(env.ErrorCode, env.Code)
	message := firstString(env.ErrorMessage, env.Message, "未知错误")
	if strings.EqualFold(env.XMLName.Local, "error") || (code != "" && code != "0" && !strings.EqualFold(code, "SUCCESS")) {
		return domain.Errorf(domain.CodeDriverError, "天翼云盘 API 错误(%s)：%s", code, message)
	}
	if out == nil {
		return nil
	}
	if err := xml.Unmarshal(data, out); err != nil {
		return domain.Wrap(domain.CodeDriverError, err)
	}
	return nil
}

func responseMessage(env map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		var s string
		if raw := env[key]; len(raw) > 0 && json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return "未知错误"
}

func successResCode(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s == "" || s == "0"
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n == 0
	}
	return false
}

func isSessionExpired(err error) bool {
	if err == nil {
		return false
	}
	if ae, ok := domain.AsAppError(err); ok && ae.Code == domain.CodeAuthExpired {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "InvalidSessionKey") || strings.Contains(msg, "userSessionBO is null")
}

func (d *Driver) signatureHeaders(method, rawURL, params string) (map[string]string, error) {
	sessionKey, sessionSecret := d.currentSession()
	if sessionKey == "" || sessionSecret == "" {
		return nil, domain.Errorf(domain.CodeAuthExpired, "天翼云盘会话未初始化")
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	return map[string]string{
		"Date":         date,
		"SessionKey":   sessionKey,
		"X-Request-ID": newRequestID(),
		"Signature":    cloud189Signature(sessionSecret, sessionKey, method, rawURL, date, params),
	}, nil
}

func cloud189Signature(secret, sessionKey, method, rawURL, date, params string) string {
	u, _ := url.Parse(rawURL)
	requestURI := "/"
	if u != nil && u.Path != "" {
		requestURI = u.Path
	}
	text := "SessionKey=" + sessionKey + "&Operate=" + strings.ToUpper(method) + "&RequestURI=" + requestURI + "&Date=" + date
	if params != "" {
		text += "&params=" + params
	}
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = io.WriteString(mac, text)
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
}

func firstString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func decodeJSON(data []byte, out any) error {
	if err := json.Unmarshal(data, out); err != nil {
		return domain.Errorf(domain.CodeDriverError, "天翼云盘返回非 JSON: %s", httpx.Truncate(data, 300))
	}
	return nil
}
