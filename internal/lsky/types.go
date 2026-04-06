package lsky

// UploadResponse LskyPro 上传响应
type UploadResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Key        string  `json:"key"`
		Name       string  `json:"name"`
		Pathname   string  `json:"pathname"`
		OriginName string  `json:"origin_name"`
		Size       float64 `json:"size"`
		Mimetype   string  `json:"mimetype"`
		Extension  string  `json:"extension"`
		Width      int     `json:"width"`
		Height     int     `json:"height"`
		MD5        string  `json:"md5"`
		SHA1       string  `json:"sha1"`
		Links      Links   `json:"links"`
	} `json:"data"`
}

// Links 响应中的链接集合
type Links struct {
	URL              string `json:"url"`
	HTML             string `json:"html"`
	BBCode           string `json:"bbcode"`
	Markdown         string `json:"markdown"`
	MarkdownWithLink string `json:"markdown_with_link"`
	ThumbnailURL     string `json:"thumbnail_url"`
}
