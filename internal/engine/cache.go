package engine

import (
	"container/list"
)

type Node struct {
	Data   string
	KeyPtr *list.Element
}

// This servers as a negative cache for absent keys
type LRUCache struct {
	Queue    *list.List
	Items    map[string]*Node
	Capacity int
}

func NewCache(capacity int) *LRUCache {
	return &LRUCache{
		Queue:    list.New(),
		Items:    make(map[string]*Node),
		Capacity: capacity,
	}
}

func (l *LRUCache) Put(key, value string) {
	if item, ok := l.Items[key]; !ok {
		if l.Capacity == len(l.Items) {
			back := l.Queue.Back()
			l.Queue.Remove(back)
			delete(l.Items, back.Value.(string))
		}
		l.Items[key] = &Node{Data: value, KeyPtr: l.Queue.PushFront(key)}
	} else {
		item.Data = value
		l.Items[key] = item
		l.Queue.MoveToFront(item.KeyPtr)
	}
}

func (l *LRUCache) Get(key string) string {
	if item, ok := l.Items[key]; ok {
		l.Queue.MoveToFront(item.KeyPtr)
		return item.Data
	}
	return ""
}

func (l *LRUCache) Delete(key string) {
	// TODO: intentionally did not evict from list. will do that later
	delete(l.Items, key)
}

func (l *LRUCache) Exists(key string) bool {
	_, ok := l.Items[key]

	return ok
}
