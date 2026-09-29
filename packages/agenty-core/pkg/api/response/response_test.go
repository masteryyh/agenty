package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/api/response"
)

func TestResponseDataPreservesJSONValues(t *testing.T) {
	for _, value := range []any{nil, "somedata", float64(0), false, []any{"one", nil}, map[string]any{"nested": nil}} {
		writer := httptest.NewRecorder()
		response.Success(writer, http.StatusCreated, value)
		var body map[string]any
		if err := json.Unmarshal(writer.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if writer.Code != http.StatusCreated {
			t.Fatalf("response status = %d, want 201", writer.Code)
		}
		want := map[string]any{"code": float64(http.StatusCreated), "message": "ok", "data": value}
		if !reflect.DeepEqual(body, want) {
			t.Fatalf("response body = %#v, want %#v", body, want)
		}
	}
}

func TestResponseFailurePreservesErrorCodeAndNullData(t *testing.T) {
	writer := httptest.NewRecorder()
	response.Failure(writer, http.StatusNotFound, "not_found", "missing resource")
	var body map[string]any
	if err := json.Unmarshal(writer.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if writer.Code != http.StatusNotFound {
		t.Fatalf("response status = %d, want 404", writer.Code)
	}
	want := map[string]any{
		"code": float64(http.StatusNotFound), "message": "missing resource", "data": nil, "errorCode": "not_found",
	}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("error response = %#v, want %#v", body, want)
	}
}

func TestResponseEncodingFailureReturnsCompleteInternalError(t *testing.T) {
	writer := httptest.NewRecorder()
	response.Success(writer, http.StatusOK, make(chan int))
	var body map[string]any
	if err := json.Unmarshal(writer.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if writer.Code != http.StatusInternalServerError {
		t.Fatalf("response status = %d, want 500", writer.Code)
	}
	want := map[string]any{
		"code": float64(http.StatusInternalServerError), "message": "internal server error", "data": nil, "errorCode": "internal",
	}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("encoding failure response = %#v, want %#v", body, want)
	}
}
