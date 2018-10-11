package whitelist

import (
	"github.com/stretchr/testify/assert"

	"testing"
)

func TestList_AddItems(t *testing.T) {

	arr := NewList().AddItems([]string{"a", "c"}).Add("b")
	assert.True(t, arr.IsSimilar([]string{"a", "b", "c"}))
	assert.Equal(t, 3, arr.Size())
}
func TestList_Contains(t *testing.T) {

	arr := NewList().AddItems([]string{"b", "a"})
	assert.True(t, arr.Contains("a"))
	assert.True(t, arr.Contains("b"))
	assert.False(t, arr.Contains("c"))
}

func TestList_IsSimilar_True(t *testing.T) {

	arr := NewList()
	arr.AddItems([]string{"a", "b", "c"})
	assert.True(t, arr.IsSimilar([]string{"b", "c", "a"}))
}

func TestList_Size(t *testing.T) {

	arr := NewList()
	arr.AddItems([]string{"a", "b", "c"})
	assert.Equal(t, 3, arr.Size())
}

func TestList_Items(t *testing.T) {

	const name = "nehmad"
	list := NewList().Add(name)
	assert.Equal(t, 1, len(list.Items()))
	assert.True(t, list.Contains(name))
}

func TestList_ItemsEmpty(t *testing.T) {

	assert.Equal(t, 0, len(NewList().Items()))
}

func TestList_IsSimilarEmptyArrays(t *testing.T) {

	arr := NewList()
	assert.True(t, arr.IsSimilar([]string{}))
}

func TestList_IsSimilarSize_False(t *testing.T) {

	arr := NewList()
	arr.AddItems([]string{"a", "c"})
	assert.False(t, arr.IsSimilar([]string{"b", "c", "a"}))
}

func TestList_IsSimilar_False(t *testing.T) {

	arr := NewList()
	arr.AddItems([]string{"a", "c", "d"})
	assert.False(t, arr.IsSimilar([]string{"b", "c", "a"}))
}
