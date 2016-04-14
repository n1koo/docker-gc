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
	//Sort map based on dates to make order predictable
	var dates []int64
	for k := range dataMap {
		dates = append(dates, k)
	}
	sort.Sort(sort.Reverse(sortutil.Int64Slice(dates)))
	return dates
}
