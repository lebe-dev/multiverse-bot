package domain

type Video struct {
	URL      string
	FilePath string
	Platform Platform
	Size     int64
	Title    string
	Channel  string
}
