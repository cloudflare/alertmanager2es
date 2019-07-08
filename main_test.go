package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestHappyPath(t *testing.T) {
	var (
		request     *http.Request
		requestBody []byte
	)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		if r.Body == nil {
			t.Fatal("got empty response body")
			return
		}
		requestBody, err = ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		request = r

		io.WriteString(w, "OK\n")
	}))
	defer s.Close()

	esURL = s.URL
	w, r := recordedRequest(t, s.URL+"/webhook")
	handler(w, r)

	expectedCode := http.StatusOK
	if w.Code != expectedCode {
		t.Fatalf("expected HTTP status %d, got %d", expectedCode, w.Code)
	}

	u, _ := url.Parse(s.URL)
	expected := u.Host
	if request.Host != expected {
		t.Fatalf("expected request host %s, got %s", expected, request.Host)
	}

	expected = "POST"
	if request.Method != expected {
		t.Fatalf("expected request method %s, got %s", expected, request.Method)
	}

	//discard ID because it changes constantly :)
	expected = fmt.Sprintf("/alertmanager-%s/alert_group", time.Now().Format("2006.01"))
	uri := strings.Join(strings.Split(request.RequestURI, "/")[:3], "/")
	if uri != expected {
		t.Fatalf("expected request path %s, got %s", expected, request.RequestURI)
	}

	// the timestamp changes every second, making it hard to test, so strip it off before comparing
	lengthOfTimestamp := 42
	l := len(esDocument) - lengthOfTimestamp

	if !bytes.Equal(esDocument[:l], requestBody[:l]) {
		t.Fatalf("expected payload %q, got %q", esDocument[:l], requestBody[:l])
	}
}

func TestErrorsPassedThrough(t *testing.T) {
	// Mock Elasticsearch server returns 404s
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()

	esURL = s.URL
	w, r := recordedRequest(t, s.URL+"/webhook")
	handler(w, r)

	expectedCode := http.StatusInternalServerError
	if w.Code != expectedCode {
		t.Fatalf("expected HTTP status %d, got %d", expectedCode, w.Code)
	}
}

func TestUserAgentSetWhenPostingToElasticsearch(t *testing.T) {
	var userAgent string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.UserAgent()
	}))
	defer s.Close()

	esURL = s.URL
	w, r := recordedRequest(t, s.URL+"/webhook")
	handler(w, r)

	expected := regexp.MustCompile(`^alertmanager2es .*? \(go\d.\d+(?:.\d+)?\)$`)
	if !expected.MatchString(userAgent) {
		t.Fatalf("expected user agent to match %s, got %q", expected.String(), userAgent)
	}

}

func recordedRequest(t *testing.T, url string) (*httptest.ResponseRecorder, *http.Request) {
	w := httptest.NewRecorder()
	r, err := http.NewRequest("POST", url, bytes.NewBuffer(amNotification))
	if err != nil {
		t.Fatal(err)
	}

	return w, r
}

var amNotification = []byte(`{
	"alerts": [
		{
			"annotations": {
				"link": "https://example.com/Foo+Bar",
				"summary": "Alert summary"
			},
			"endsAt": "0001-01-01T00:00:00Z",
			"generatorURL": "https://example.com",
			"labels": {
				"alertname": "Foo_Bar",
				"instance": "foo"
			},
			"startsAt": "2017-02-02T16:51:13.507955756Z",
			"status": "firing"
		}
	],
	"commonAnnotations": {
		"link": "https://example.com/Foo+Bar",
		"summary": "Alert summary"
	},
	"commonLabels": {
		"alertname": "Foo_Bar",
		"instance": "foo"
	},
	"externalURL": "https://alertmanager.example.com",
	"groupLabels": {
		"alertname": "Foo_Bar"
	},
	"receiver": "alertmanager2es",
	"status": "firing",
	"version": "4",
	"groupKey": "{}/{}/{notify=\"default\":{alertname=\"Foo_Bar\", instance=\"foo\"}"
}`)

var esDocument = []byte(`{"annotations":{"link":"https://example.com/Foo+Bar","summary":"Alert summary"},"endsAt":"0001-01-01T00:00:00Z","generatorURL":"https://example.com","labels":{"alertname":"Foo_Bar","instance":"foo"},"startsAt":"2017-02-02T16:51:13.507955756Z","status":"firing","@timestamp":"2017-02-02T19:37:22+01:00"}`)
