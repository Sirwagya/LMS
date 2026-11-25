package envelope

import "time"

type ApiResponse[T any] struct {
	Data        T      `json:"data"`
	ErrorCode   any    `json:"errorCode"`
	ErrorPath   any    `json:"errorPath"`
	FieldErrors any    `json:"fieldErrors"`
	Message     string `json:"message"`
	StatusCode  int    `json:"statusCode"`
	Timestamp   int64  `json:"timestamp"`
}

func Success[T any](data T, statusCode int, now time.Time) ApiResponse[T] {
	return ApiResponse[T]{
		Data:        data,
		ErrorCode:   nil,
		ErrorPath:   nil,
		FieldErrors: nil,
		Message:     "Success",
		StatusCode:  statusCode,
		Timestamp:   now.UnixMilli(),
	}
}

type Page[T any] struct {
	Items            []T   `json:"items"`
	Page             int   `json:"page"`
	Size             int   `json:"size"`
	TotalElements    int64 `json:"totalElements"`
	TotalPages       int   `json:"totalPages"`
	NumberOfElements int   `json:"numberOfElements"`
	First            bool  `json:"first"`
	Last             bool  `json:"last"`
	Meta             any   `json:"meta"`
}

func NewPage[T any](items []T, page, size int, total int64) Page[T] {
	if items == nil {
		items = []T{}
	}
	if size <= 0 {
		size = 100
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(size) - 1) / int64(size))
	}
	return Page[T]{
		Items:            items,
		Page:             page,
		Size:             size,
		TotalElements:    total,
		TotalPages:       totalPages,
		NumberOfElements: len(items),
		First:            page == 0,
		Last:             totalPages == 0 || page >= totalPages-1,
		Meta:             nil,
	}
}
