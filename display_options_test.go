package main

import (
	"strings"
	"testing"
)

func TestDisplayRedLettersAndWordAnnotations(t *testing.T) {
	input := `<q who="Jesus">Listen</q> <w lemma="λόγος" morph="N-NSM" xlit="logos">word</w>`
	filters := DefaultFilters
	on := ParseOSISVerse(input, filters)
	if !strings.Contains(on.Text, `class="red-letter"`) {
		t.Fatal("missing red letters")
	}
	filters.ShowRedLetters = false
	off := ParseOSISVerse(input, filters)
	if strings.Contains(off.Text, "red-letter") {
		t.Fatal("red letters remain disabled")
	}
	if len(on.StrongsNumbers) != 1 || on.StrongsNumbers[0].Morph != "N-NSM" || on.StrongsNumbers[0].Lemma != "λόγος" || on.StrongsNumbers[0].Xlit != "logos" {
		t.Fatalf("missing word annotations: %+v", on.StrongsNumbers)
	}
}
