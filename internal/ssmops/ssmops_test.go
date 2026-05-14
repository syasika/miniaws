package ssmops

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/syasika/miniaws/internal/awsclient"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *ssm.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return awsclient.NewSSMClient(awsclient.NewConfig(), srv.URL)
}

func target(name string) string {
	return "AmazonSSM." + name
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code, msg string) {
	w.WriteHeader(http.StatusBadRequest)
	writeJSON(w, map[string]string{"__type": code, "Message": msg})
}

func readBody(r *http.Request) string {
	data, _ := io.ReadAll(r.Body)
	r.Body.Close()
	return string(data)
}

// --- IsConnectionErr ---

func TestIsConnectionErr(t *testing.T) {
	if got := IsConnectionErr(nil); got != false {
		t.Errorf("IsConnectionErr(nil) = %v, want false", got)
	}
}

// --- ListAllParameters ---

func TestListAllParameters(t *testing.T) {
	var callCount int
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != target("DescribeParameters") {
			t.Errorf("unexpected target: %s", r.Header.Get("X-Amz-Target"))
		}
		callCount++
		if callCount == 1 {
			writeJSON(w, map[string]interface{}{
				"Parameters": []map[string]interface{}{
					{"Name": "alpha", "Type": "String", "LastModifiedDate": 1.7040672e9, "Version": 1.0},
					{"Name": "beta", "Type": "StringList", "LastModifiedDate": 1.7040672e9, "Version": 2.0},
				},
				"NextToken": "tok1",
			})
			return
		}
		writeJSON(w, map[string]interface{}{
			"Parameters": []map[string]interface{}{
				{"Name": "gamma", "Type": "SecureString", "LastModifiedDate": 1.7040672e9, "Version": 3.0},
			},
		})
	})

	params, err := ListAllParameters(context.Background(), client)
	if err != nil {
		t.Fatalf("ListAllParameters: %v", err)
	}
	if len(params) != 3 {
		t.Fatalf("got %d params, want 3", len(params))
	}
	if params[0].Name != "alpha" || params[0].Type != "String" || params[0].Version != 1 {
		t.Errorf("params[0] = %+v", params[0])
	}
	if params[1].Name != "beta" || params[1].Type != "StringList" || params[1].Version != 2 {
		t.Errorf("params[1] = %+v", params[1])
	}
	if params[2].Name != "gamma" || params[2].Type != "SecureString" || params[2].Version != 3 {
		t.Errorf("params[2] = %+v", params[2])
	}
}

func TestListAllParametersError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "InternalServerError", "something broke")
	})
	_, err := ListAllParameters(context.Background(), client)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAllParametersEmpty(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"Parameters": []interface{}{}})
	})
	params, err := ListAllParameters(context.Background(), client)
	if err != nil {
		t.Fatalf("ListAllParameters: %v", err)
	}
	if len(params) != 0 {
		t.Errorf("got %d params, want 0", len(params))
	}
}

// --- ListParameters (paged) ---

func TestListParametersFirstPage(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.Unmarshal([]byte(readBody(r)), &body)
		if _, has := body["NextToken"]; has {
			t.Errorf("unexpected NextToken in first page request")
		}
		writeJSON(w, map[string]interface{}{
			"Parameters": []map[string]interface{}{
				{"Name": "page1", "Type": "String", "LastModifiedDate": 1.7040672e9, "Version": 1.0},
			},
			"NextToken": "next-page",
		})
	})
	page, err := ListParameters(context.Background(), client, nil, 20)
	if err != nil {
		t.Fatalf("ListParameters: %v", err)
	}
	if len(page.Parameters) != 1 || page.Parameters[0].Name != "page1" {
		t.Errorf("page.Parameters = %+v", page.Parameters)
	}
	if page.NextToken == nil || *page.NextToken != "next-page" {
		t.Errorf("NextToken = %v, want next-page", page.NextToken)
	}
}

func TestListParametersWithToken(t *testing.T) {
	tok := "abc"
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.Unmarshal([]byte(readBody(r)), &body)
		if body["NextToken"] != "abc" {
			t.Errorf("NextToken = %v, want abc", body["NextToken"])
		}
		writeJSON(w, map[string]interface{}{
			"Parameters": []map[string]interface{}{
				{"Name": "p2", "Type": "String", "LastModifiedDate": 1.7040672e9, "Version": 2.0},
			},
		})
	})
	page, err := ListParameters(context.Background(), client, &tok, 10)
	if err != nil {
		t.Fatalf("ListParameters: %v", err)
	}
	if len(page.Parameters) != 1 || page.Parameters[0].Name != "p2" {
		t.Errorf("page.Parameters = %+v", page.Parameters)
	}
	if page.NextToken != nil {
		t.Errorf("NextToken = %v, want nil", page.NextToken)
	}
}

func TestListParametersError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "ValidationException", "bad request")
	})
	_, err := ListParameters(context.Background(), client, nil, 20)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- GetParameter ---

func TestGetParameter(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != target("GetParameter") {
			t.Errorf("unexpected target: %s", r.Header.Get("X-Amz-Target"))
		}
		writeJSON(w, map[string]interface{}{
			"Parameter": map[string]interface{}{
				"Name":             "my-param",
				"Type":             "String",
				"Value":            "hello",
				"LastModifiedDate": 1.7040672e9,
				"Version":          5.0,
			},
		})
	})

	p, err := GetParameter(context.Background(), client, "my-param")
	if err != nil {
		t.Fatalf("GetParameter: %v", err)
	}
	if p.Name != "my-param" || p.Value != "hello" || p.Type != "String" || p.Version != 5 {
		t.Errorf("Parameter = %+v", p)
	}
}

func TestGetParameterError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "ParameterNotFound", "param not found")
	})
	_, err := GetParameter(context.Background(), client, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- PutParameter ---

func TestPutParameter(t *testing.T) {
	var callTarget string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callTarget = r.Header.Get("X-Amz-Target")
		var body map[string]interface{}
		json.Unmarshal([]byte(readBody(r)), &body)
		if body["Name"] != "/cfg/key" || body["Value"] != "val" || body["Type"] != "String" {
			t.Errorf("body = %v", body)
		}
		writeJSON(w, map[string]interface{}{})
	})

	if err := PutParameter(context.Background(), client, "/cfg/key", "val", "String"); err != nil {
		t.Fatalf("PutParameter: %v", err)
	}
	if callTarget != target("PutParameter") {
		t.Errorf("target = %s", callTarget)
	}
}

func TestPutParameterError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "ParameterLimitExceeded", "too many")
	})
	err := PutParameter(context.Background(), client, "x", "y", "String")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- DeleteParameter ---

func TestDeleteParameter(t *testing.T) {
	var callTarget string
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callTarget = r.Header.Get("X-Amz-Target")
		var body map[string]interface{}
		json.Unmarshal([]byte(readBody(r)), &body)
		if body["Name"] != "/old/key" {
			t.Errorf("body = %v", body)
		}
		writeJSON(w, map[string]interface{}{})
	})

	if err := DeleteParameter(context.Background(), client, "/old/key"); err != nil {
		t.Fatalf("DeleteParameter: %v", err)
	}
	if callTarget != target("DeleteParameter") {
		t.Errorf("target = %s", callTarget)
	}
}

func TestDeleteParameterError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, "ParameterNotFound", "not there")
	})
	err := DeleteParameter(context.Background(), client, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Connection error handling ---

func TestConnectionErrorFromList(t *testing.T) {
	client := awsclient.NewSSMClient(awsclient.NewConfig(), "http://127.0.0.1:1")
	_, err := ListAllParameters(context.Background(), client)
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "cannot reach ministack") {
		t.Errorf("error = %v, want friendly message", err)
	}
}

func TestListAllParametersFriendlyAPIError(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"__type":"AmazonSSM.ValidationException","Message":"bad param"}`))
	})
	_, err := ListAllParameters(context.Background(), client)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "ssm api error") {
		t.Errorf("error = %v, want friendly API error", err)
	}
}
