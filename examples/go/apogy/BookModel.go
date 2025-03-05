package apogy

// BookVal represents a generated struct
type BookVal struct {
	Name string `json:"name"`
	Reviews []BookValReviews `json:"reviews,omitempty"`
	Author string `json:"author"`
	Isbn *string `json:"isbn,omitempty"`
}

// BookValReviews represents a generated struct
type BookValReviews struct {
	Text string `json:"text"`
	Author string `json:"author"`
}

