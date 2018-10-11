package whitelist

type List struct {
	// Using an empty struct{} has advantage that it doesn't require any additional space
	// Go's internal map type is optimized for that kind of values
	data map[string]struct{}
}

func NewList() *List {

	return &List{data: make(map[string]struct{})}
}

func (arr *List) AddItems(items []string) *List {

	for _, curr := range items {
		arr.Add(curr)
	}

	return arr
}

func (arr *List) Add(item string) *List {

	arr.data[item] = struct{}{}

	return arr
}

func (arr List) Contains(item string) bool {

	_, ret := arr.data[item]

	return ret
}

func (arr List) Size() int {

	return len(arr.data)
}

func (arr List) Items() []string {

	if len(arr.data) == 0 {
		return []string{}
	}

	ret := make([]string, 0, len(arr.data))
	for i := range arr.data {
		ret = append(ret, i)
	}

	return ret
}

// IsSimilar returns true if both arrays contains same items
func (arr List) IsSimilar(list []string) bool {

	ret := true
	if arr.Size() != len(list) {
		ret = false
	} else {
		for _, currItem := range list {
			if !arr.Contains(currItem) {
				ret = false
				break
			}
		}
	}

	return ret
}
