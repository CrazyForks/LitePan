package guangya

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"litepan/internal/driver"
)

type qrRoundTripper func(*http.Request) (*http.Response, error)

func (fn qrRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func qrJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestQRSessionRoundTrip(t *testing.T) {
	want := qrSession{
		DeviceCode: "device-code",
		DeviceID:   "0123456789abcdef0123456789abcdef",
		ClientID:   defaultClientID,
		Created:    time.Now().Unix(),
		ExpiresIn:  120,
	}
	token, err := encodeQRSession(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeQRSession(token)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("decoded session = %#v, want %#v", got, want)
	}
}

func TestDecodeQRSessionRejectsInvalidToken(t *testing.T) {
	if _, err := decodeQRSession("not-a-session"); err == nil {
		t.Fatal("expected invalid session token error")
	}
}

func TestStartQRLoginUsesDeviceAuthorization(t *testing.T) {
	d := &Driver{client: &http.Client{Transport: qrRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/auth/device/code" {
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		if req.Header.Get("X-Device-Id") == "" {
			t.Fatal("missing device id header")
		}
		return qrJSONResponse(http.StatusOK, `{
			"device_code":"device-code","expires_in":120,"interval":2,
			"verification_uri_complete":"https://account.guangyapan.com/__/auth/device/?user_code=test"
		}`), nil
	})}}
	result, err := d.StartQRLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.QRImageBase64 == "" || result.ExpiresIn != 120 {
		t.Fatalf("unexpected start result: %#v", result)
	}
}

func TestPollQRLoginPending(t *testing.T) {
	sess := qrSession{
		DeviceCode: "device-code", DeviceID: "0123456789abcdef0123456789abcdef",
		ClientID: defaultClientID, Created: time.Now().Unix(), ExpiresIn: 120,
	}
	token, err := encodeQRSession(sess)
	if err != nil {
		t.Fatal(err)
	}
	d := &Driver{client: &http.Client{Transport: qrRoundTripper(func(*http.Request) (*http.Response, error) {
		return qrJSONResponse(http.StatusBadRequest, `{"error":"authorization_pending"}`), nil
	})}}
	result, err := d.PollQRLogin(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != driver.QRWaiting {
		t.Fatalf("status = %s, want waiting", result.Status)
	}
}

func TestPollQRLoginSuccessReturnsDeviceFields(t *testing.T) {
	sess := qrSession{
		DeviceCode: "device-code", DeviceID: "0123456789abcdef0123456789abcdef",
		ClientID: defaultClientID, Created: time.Now().Unix(), ExpiresIn: 120,
	}
	token, err := encodeQRSession(sess)
	if err != nil {
		t.Fatal(err)
	}
	d := &Driver{client: &http.Client{Transport: qrRoundTripper(func(*http.Request) (*http.Response, error) {
		return qrJSONResponse(http.StatusOK, `{"access_token":"access","refresh_token":"refresh"}`), nil
	})}}
	result, err := d.PollQRLogin(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != driver.QRSuccess || result.Credentials.RefreshToken != "refresh" {
		t.Fatalf("unexpected poll result: %#v", result)
	}
	if result.Fields["device_id"] != sess.DeviceID || result.Fields["client_id"] != defaultClientID {
		t.Fatalf("unexpected config fields: %#v", result.Fields)
	}
}
