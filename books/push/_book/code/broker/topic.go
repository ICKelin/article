package broker

import "fmt"

type Topic struct {
	Key string
}

func (t *Topic) String() string {
	return t.Key
}
