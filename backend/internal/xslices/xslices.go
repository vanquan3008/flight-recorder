// Package xslices holds the slice helpers the standard library does not have.
package xslices

// Map converts every item. The result is never nil, so an empty input encodes as an empty JSON array.
func Map[Item, Result any](items []Item, convert func(Item) Result) []Result {
	results := make([]Result, 0, len(items))
	for _, item := range items {
		results = append(results, convert(item))
	}
	return results
}
