package iplookup

import (
	"fmt"
	"net/http"
	"time"
)

// Server the cache
func (ipl *IPLookup) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	lookup := r.URL.String()
	if len(lookup) > 0 && lookup[0] == '/' {
		lookup = lookup[1:]
	}

	ipl.Lock()
	entry, _ := ipl.cacheLookup[lookup]
	ipl.Unlock()

	if entry == nil {
		http.NotFound(w, r)
		return
	}

	// Write the entry
	w.Write([]byte(entry.value))
}

func (ipl *IPLookup) addCache(name, value string) {

	ipl.Lock()

	// See if we know about this entry
	entry, found := ipl.cacheLookup[name]
	if found {

		// Update the expires time
		entry.expires = time.Now().Add(ipl.maxDuration)

		// We only need to modify if we're not the head already
		if entry.next != nil {

			// We are the tail, we know we're not the head
			if entry.prev == nil {
				ipl.tail = entry.next
				entry.next.prev = nil
			} else {
				// We know we're not the tail and not the head
				entry.prev.next = entry.next
				entry.next.prev = entry.prev
			}

			ipl.head.next = entry
			entry.prev = ipl.head
			entry.next = nil
			ipl.head = entry

		}

	} else {

		// Doesn't exist, add it to the cache
		entry = &cacheEntry{
			name:    name,
			value:   value,
			expires: time.Now().Add(ipl.maxDuration),

			next: nil,
			prev: ipl.head,
		}

		if ipl.head != nil {
			ipl.head.next = entry
		} else {
			ipl.tail = entry // Head is nil, so is tail, set it
		}
		ipl.head = entry
		ipl.cacheLookup[name] = entry

		ipl.size++

		// Make sure we haven't exceeded the number of entries
		if ipl.maxEntries > 0 && ipl.size > ipl.maxEntries {
			ipl.cleanCache()
		}

	}

	ipl.Unlock()

}

// Clean the cache, it must be locked
func (ipl *IPLookup) cleanCache() {

	now := time.Now()

	// We trim the cache from the tail forward until we get to the right size or the right duration
	for ipl.tail != nil {

		// If we have a maxDuration set and the tail is after that
		// If we have maxEntries set and we're more than that
		if (ipl.maxDuration > 0 && now.After(ipl.tail.expires)) || (ipl.maxEntries > 0 && ipl.size > ipl.maxEntries) {

			// Update the tail next.prev
			if ipl.tail.next != nil {
				ipl.tail.next.prev = nil
			}

			// Delete from the lookup
			delete(ipl.cacheLookup, ipl.tail.name)

			// Advance the tail pointer
			ipl.tail = ipl.tail.next
			if ipl.tail == nil {
				ipl.head = nil
			}

			ipl.size--

		} else {
			// We have trimmed everything off the list that is required
			break
		}

	}

	fmt.Printf("F: %+v\n", ipl)

}
