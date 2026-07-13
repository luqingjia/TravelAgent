package httpapi

const (
	SuccessCode      = "0"
	ClientErrorCode  = "A000001"
	ServiceErrorCode = "B000001"
)

type Result struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func Success(data any) Result {
	return Result{Code: SuccessCode, Data: data}
}

func Failure(code string, message string) Result {
	return Result{Code: code, Message: message}
}
