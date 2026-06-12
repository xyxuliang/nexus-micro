package dto

type Payment struct {
	ID      int64   `json:"id"`
	OrderId int64   `json:"order_id"`
	Amount  float64 `json:"amount"`
	Method  string  `json:"method"`
	Status  string  `json:"status"`
	Created int64   `json:"created"`
}
type CreatePaymentReq struct {
	OrderId int64   `json:"order_id"`
	Amount  float64 `json:"amount"`
	Method  string  `json:"method"`
}
type CreatePaymentResp struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}
type GetPaymentReq struct{ ID int64 }
type GetPaymentResp struct {
	Payment Payment `json:"payment"`
}
