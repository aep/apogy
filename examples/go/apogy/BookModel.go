package apogy

// BookVal represents a generated struct
type BookVal struct {
	Isbn *string `json:"isbn,omitempty"`
	Name string `json:"name"`
	Reviews []BookValReviews `json:"reviews,omitempty"`
	Author string `json:"author"`
}

// BookValReviews represents a generated struct
type BookValReviews struct {
	Author string `json:"author"`
	Text string `json:"text"`
}

