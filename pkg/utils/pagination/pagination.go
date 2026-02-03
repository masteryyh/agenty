package pagination

type PagedResponse[T any] struct {
	Total    int64 `json:"total"`
	PageSize int   `json:"pageSize"`
	Page     int   `json:"page"`
	Data     []T   `json:"data"`
}

type OrderType string

const (
	OrderTypeAscending  OrderType = "asc"
	OrderTypeDescending OrderType = "desc"
)

type PageRequest struct {
	Page     int       `query:"page" json:"page" validate:"required,min=1"`
	PageSize int       `query:"pageSize" json:"pageSize" validate:"required,min=1,max=100"`
}

func (p *PageRequest) ApplyDefaults() {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = 10
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
}
