package httpapi

import "testing"

func TestSuccessMatchesJavaResultShape(t *testing.T) {
	result := Success("ok")
	if result.Code != "0" {
		t.Fatalf("success code = %q, want 0", result.Code)
	}
	if result.Message != "" {
		t.Fatalf("success message = %q, want empty", result.Message)
	}
	if result.Data != "ok" {
		t.Fatalf("success data = %v, want ok", result.Data)
	}
}

func TestFailureUsesProjectErrorCode(t *testing.T) {
	result := Failure("A000001", "参数错误")
	if result.Code != "A000001" {
		t.Fatalf("failure code = %q, want A000001", result.Code)
	}
	if result.Message != "参数错误" {
		t.Fatalf("failure message = %q, want 参数错误", result.Message)
	}
	if result.Data != nil {
		t.Fatalf("failure data should be nil")
	}
}
