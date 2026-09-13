package page

const (
	DefaultPageNum  = 1
	DefaultPageSize = 20
	MaxPageSize     = 200
)

type Request struct {
	PageNum  int `json:"pageNum"`
	PageSize int `json:"pageSize"`
}

func (r *Request) Normalize() {
	if r.PageNum <= 0 {
		r.PageNum = DefaultPageNum
	}
	if r.PageSize <= 0 {
		r.PageSize = DefaultPageSize
	}
	if r.PageSize > MaxPageSize {
		r.PageSize = MaxPageSize
	}
}

type Response[T any] struct {
	Total int64 `json:"total"`
	List  []T   `json:"list"`
}
