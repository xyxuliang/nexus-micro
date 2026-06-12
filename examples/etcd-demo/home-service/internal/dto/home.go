package dto

type Banner struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Image string `json:"image"`
	Url   string `json:"url"`
	Sort  int    `json:"sort"`
}
type HomeResp struct {
	Banners []Banner `json:"banners"`
	Message string   `json:"message"`
}
