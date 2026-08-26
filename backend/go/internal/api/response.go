package api

const Version = 1

type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Response struct {
	Success bool            `json:"success"`
	API     int             `json:"api"`
	Data    *map[string]any `json:"data,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

func Success(data map[string]any) Response {
	if data == nil {
		data = map[string]any{}
	}

	return Response{
		Success: true,
		API:     Version,
		Data:    &data,
	}
}

func Failure(code string, message string) Response {
	return Response{
		Success: false,
		API:     Version,
		Error: &Error{
			Code:    code,
			Message: message,
		},
	}
}
