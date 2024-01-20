package composeloader

type ProjectDir[T any] struct {
}

func (d *ProjectDir[T]) ComposeYmlPath() string {
	return ""
}
