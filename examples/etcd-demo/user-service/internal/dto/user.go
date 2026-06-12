package dto

type User struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Created int64  `json:"created"`
}

type CreateUserReq struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}
type CreateUserResp struct {
	ID int64 `json:"id"`
}
type GetUserReq struct{ ID int64 }
type GetUserResp struct {
	User User `json:"user"`
}
