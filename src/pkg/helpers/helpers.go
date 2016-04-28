package helpers

import (
	"sort"

	"github.com/cznic/sortutil"
)

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func SortDataMap(dataMap map[int64][]string) []int64 {
	//Sort map keys to make order predictable for indexing
	keys := getKeysFromMap(dataMap)
	sort.Sort(sortutil.Int64Slice(keys))
	return keys
}

func SortDataMapReverse(dataMap map[int64][]string) []int64 {
	//Sort map keys to make order predictable for indexing (REVERSE)
	keys := getKeysFromMap(dataMap)
	sort.Sort(sort.Reverse(sortutil.Int64Slice(keys)))
	return keys
}

func getKeysFromMap(dataMap map[int64][]string) []int64 {
	var keys []int64
	for k := range dataMap {
		keys = append(keys, k)
	}
	return keys
}
