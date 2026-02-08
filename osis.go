package main

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// OSISFilters represents which OSIS filters are enabled
type OSISFilters struct {
	ShowStrongs    bool // Strong's numbers
	ShowFootnotes  bool // Footnotes
	ShowScripref   bool // Scripture cross-references
	ShowHeadings   bool // Section headings
	ShowRedLetters bool // Red letter words (words of Jesus)
	ShowLemma      bool // Original language lemmas
	ShowMorph      bool // Morphological information
	ShowXlit       bool // Transliterations
}

// Default filters - most features shown by default
var DefaultFilters = OSISFilters{
	ShowStrongs:    false, // Off by default - can be overwhelming
	ShowFootnotes:  true,
	ShowScripref:   true,
	ShowHeadings:   true,
	ShowRedLetters: true,
	ShowLemma:      false,
	ShowMorph:      false,
	ShowXlit:       false,
}

// OSIS tag patterns
var (
	// Word markup with strongs/lemma/morph
	wordTagPattern = regexp.MustCompile(`<w\s+([^>]+)>([^<]*)</w>`)

	// Notes (footnotes, cross-references, etc.)
	noteTagPattern = regexp.MustCompile(`<note\s+([^>]*?)(?:type="([^"]+)")?([^>]*)>(.*?)</note>`)

	// Divine name markup (LORD, GOD, etc.)
	divineNamePattern = regexp.MustCompile(`<divineName>([^<]+)</divineName>`)

	// Reference links
	refTagPattern = regexp.MustCompile(`<reference[^>]*osisRef="([^"]+)"[^>]*>([^<]+)</reference>`)

	// Emphasized text
	hiPattern = regexp.MustCompile(`<hi\s+type="([^"]+)">([^<]+)</hi>`)

	// Transliteration
	xlitPattern = regexp.MustCompile(`<w[^>]*xlit="([^"]+)"[^>]*>([^<]*)</w>`)

	// Quote markers
	qTagPattern    = regexp.MustCompile(`<q\s+([^>]*)>`)
	qEndTagPattern = regexp.MustCompile(`</q>`)

	// Catch words in notes
	catchWordTagPattern = regexp.MustCompile(`<catchWord>([^<]+)</catchWord>`)

	// Trans change (added words)
	transChangeTagPattern = regexp.MustCompile(`<transChange\s+type="added">([^<]*)</transChange>`)

	// Line breaks
	lbPattern = regexp.MustCompile(`<lb\s*/?>`)

	// Milestones (paragraph markers, etc.)
	milestoneTagPattern = regexp.MustCompile(`<milestone[^>]*/>`)

	// Verse markers
	verseTagPattern = regexp.MustCompile(`<verse[^>]*/>`)

	// Chapter markers
	chapterTagPattern = regexp.MustCompile(`<chapter[^>]*/>`)

	// Title patterns (already exist in main.go but included here for completeness)
	titleSectionPattern  = regexp.MustCompile(`<title[^>]*(?:subType="x-preverse"|type="x-s")[^>]*>([^<]+)</title>`)
	titleDescPattern     = regexp.MustCompile(`<title[^>]*type="x-description"[^>]*>([^<]+)</title>`)
	titleParallelPattern = regexp.MustCompile(`<title[^>]*type="parallel"[^>]*>(.*?)</title>`)
	titleGenericPattern  = regexp.MustCompile(`<title>([^<]+)</title>`)
)

// ParsedVerse represents a parsed OSIS verse with structured data
type ParsedVerse struct {
	Text           string              `json:"text"`           // Main verse text
	SectionTitle   string              `json:"sectionTitle"`   // Section heading before verse
	Footnotes      []Footnote          `json:"footnotes"`      // All footnotes
	CrossRefs      []CrossReference    `json:"crossRefs"`      // Cross-references
	StrongsNumbers []StrongsAnnotation `json:"strongsNumbers"` // Strong's number annotations
	HasRedLetters  bool                `json:"hasRedLetters"`  // Whether verse contains words of Jesus
	RawOSIS        string              `json:"rawOSIS"`        // Original OSIS markup
	DebugPass1     string              `json:"debugPass1,omitempty"`
	DebugPass2     string              `json:"debugPass2,omitempty"`
	DebugTagText   string              `json:"debugTagText,omitempty"`
}

// Footnote represents a footnote in the text
type Footnote struct {
	Marker string `json:"marker"` // Footnote marker (a, b, c, etc.)
	Text   string `json:"text"`   // Footnote content
	Type   string `json:"type"`   // Type: explanation, study, variant, etc.
}

// CrossReference represents a scripture cross-reference
type CrossReference struct {
	Marker     string   `json:"marker"`     // Reference marker
	References []string `json:"references"` // List of verse references
	Text       string   `json:"text"`       // Display text
}

// StrongsAnnotation represents Strong's number markup
type StrongsAnnotation struct {
	Word    string   `json:"word"`    // The word in the text
	Strongs []string `json:"strongs"` // Strong's number(s)
	Lemma   string   `json:"lemma"`   // Dictionary form
	Morph   string   `json:"morph"`   // Morphology
	Xlit    string   `json:"xlit"`    // Transliteration
}

// ParseOSISVerse parses an OSIS verse with the given filters
func ParseOSISVerse(osisText string, filters OSISFilters) ParsedVerse {
	// Use a streaming XML-based parser to avoid fragile regex-based token handling.
	return parseOSISUsingXML(osisText, filters)
}

// parseOSISUsingXML implements a streaming XML parse of OSIS markup.
func parseOSISUsingXML(osisText string, filters OSISFilters) ParsedVerse {
	result := ParsedVerse{
		RawOSIS:        osisText,
		Footnotes:      []Footnote{},
		CrossRefs:      []CrossReference{},
		StrongsNumbers: []StrongsAnnotation{},
	}

	dec := xml.NewDecoder(strings.NewReader(osisText))
	var out bytes.Buffer
	footnoteNum := 1
	crossRefCounter := 0
	// track nested <q who="..."> contexts for red-letter handling
	redWhoStack := []string{}
	redSpanOpen := false
	skipQEnd := 0

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			// Handle start of quoted speech (<q who="...">) by pushing context
			if strings.EqualFold(name, "q") {
				who := ""
				hasSID := false
				hasEID := false
				for _, a := range t.Attr {
					if a.Name.Local == "who" {
						who = a.Value
					}
					if a.Name.Local == "sID" {
						hasSID = true
					}
					if a.Name.Local == "eID" {
						hasEID = true
					}
				}
				if hasSID {
					// start marker (often self-closing) — push context and open span
					redWhoStack = append(redWhoStack, who)
					if who != "" && filters.ShowRedLetters {
						out.WriteString(`<span class="red-letter">`)
						redSpanOpen = true
						result.HasRedLetters = true
					}
					skipQEnd++
					continue
				}
				if hasEID {
					// end marker (often self-closing) — pop context and close span
					if len(redWhoStack) > 0 {
						redWhoStack = redWhoStack[:len(redWhoStack)-1]
					}
					if redSpanOpen {
						out.WriteString(`</span>`)
						redSpanOpen = false
					}
					skipQEnd++
					continue
				}
				// regular <q> with inner content
				redWhoStack = append(redWhoStack, who)
				if who != "" && filters.ShowRedLetters {
					out.WriteString(`<span class="red-letter">`)
					redSpanOpen = true
					result.HasRedLetters = true
				}
				continue
			}
			// Handle word elements with lemma/xlit/strongs
			if strings.EqualFold(name, "w") {
				// collect attributes
				lemmaVal := ""
				xlitVal := ""
				for _, a := range t.Attr {
					if a.Name.Local == "lemma" {
						lemmaVal = a.Value
					}
					if a.Name.Local == "xlit" {
						xlitVal = a.Value
					}
				}

				// capture inner text of <w>
				var innerBuf bytes.Buffer
				depth := 1
				for depth > 0 {
					nt, err := dec.Token()
					if err != nil {
						break
					}
					switch v := nt.(type) {
					case xml.CharData:
						innerBuf.Write([]byte(v))
					case xml.StartElement:
						depth++
					case xml.EndElement:
						depth--
					}
				}
				wordText := strings.TrimSpace(innerBuf.String())
				// write the visible word to output
				out.WriteString(wordText)

				// extract Strong's numbers from lemma attribute (e.g., "strong:G976" or "strong:G1544b")
				// normalize to upper-case G#### without trailing qualifiers
				strongs := []string{}
				if lemmaVal != "" {
					strongRe := regexp.MustCompile(`(?i)\bG(\d+)[A-Za-z]?\b`)
					for _, sub := range strongRe.FindAllStringSubmatch(lemmaVal, -1) {
						if len(sub) > 1 {
							id := "G" + sub[1]
							strongs = append(strongs, strings.ToUpper(id))
						}
					}
				}

				// clean lemma: remove Strong's tokens (strong: or bare G###) and known prefixes
				cleanLemma := lemmaVal
				if cleanLemma != "" {
					// remove tokens like "strong:G123" or "strongs:G123b" (allow trailing qualifier)
					cleanLemma = regexp.MustCompile(`(?i)(?:strong|strongs):?G\d+[A-Za-z]?`).ReplaceAllString(cleanLemma, "")
					// remove any remaining bare G### occurrences with optional qualifier
					cleanLemma = regexp.MustCompile(`(?i)\bG\d+[A-Za-z]?\b`).ReplaceAllString(cleanLemma, "")
					cleanLemma = strings.TrimSpace(cleanLemma)
					// strip common module prefixes
					cleanLemma = strings.TrimPrefix(cleanLemma, "lemma.")
					cleanLemma = strings.TrimPrefix(cleanLemma, "BSBlex:")
				}

				if len(strongs) > 0 || cleanLemma != "" || xlitVal != "" {
					finalLemma := cleanLemma
					finalXlit := xlitVal
					// If the original `lemma` attribute existed but cleaned to empty
					// (i.e. it only contained Strong's tokens), we assume this
					// translation does not provide lemma text and should not
					// be supplemented from a hard-coded lexicon. Only attempt
					// a lexicon lookup when there was no `lemma` attribute
					// at all.
					if finalLemma == "" && len(strongs) > 0 && lemmaVal == "" {
						// lexicon lookup would go here if enabled
					}
					// Only expose lemma/xlit when the original <w lemma="..."> attribute
					// provided lemma text. If the lemma was derived from a lexicon lookup
					// (i.e., original `lemmaVal` was empty), do not populate `Lemma` here.
					outLemma := ""
					outXlit := ""
					if lemmaVal != "" {
						outLemma = finalLemma
						outXlit = finalXlit
					}
					result.StrongsNumbers = append(result.StrongsNumbers, StrongsAnnotation{
						Word:    wordText,
						Strongs: strongs,
						Lemma:   outLemma,
						Xlit:    outXlit,
					})
				}

				continue
			}
			// Title/section capture
			if strings.EqualFold(name, "title") {
				// inspect attributes to decide how to handle the title
				titleType := ""
				titleSubType := ""
				for _, a := range t.Attr {
					if a.Name.Local == "type" {
						titleType = a.Value
					}
					if a.Name.Local == "subType" {
						titleSubType = a.Value
					}
				}

				// If this is a parallel title (type="parallel"), do not treat it as a section heading
				if strings.EqualFold(titleType, "parallel") {
					// consume and ignore inner tokens
					depth := 1
					for depth > 0 {
						nt, err := dec.Token()
						if err != nil {
							break
						}
						switch nt.(type) {
						case xml.StartElement:
							depth++
						case xml.EndElement:
							depth--
						}
					}
					continue
				}

				// capture inner text for section title if headings enabled and this looks like a section title
				if filters.ShowHeadings && (strings.EqualFold(titleSubType, "x-preverse") || strings.EqualFold(titleType, "x-s") || titleType == "") {
					var buf bytes.Buffer
					enc := xml.NewEncoder(&buf)
					depth := 1
					for depth > 0 {
						nt, err := dec.Token()
						if err != nil {
							break
						}
						switch nt.(type) {
						case xml.StartElement:
							depth++
						case xml.EndElement:
							depth--
						}
						if depth > 0 {
							enc.EncodeToken(nt)
						}
					}
					enc.Flush()
					// strip inner tags
					titleText := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(buf.String(), "")
					result.SectionTitle = strings.TrimSpace(titleText)
				} else {
					// skip title contents
					depth := 1
					for depth > 0 {
						nt, err := dec.Token()
						if err != nil {
							break
						}
						switch nt.(type) {
						case xml.StartElement:
							depth++
						case xml.EndElement:
							depth--
						}
					}
				}
				continue
			}

			if strings.EqualFold(name, "note") {
				// collect attributes
				marker := ""
				noteType := ""
				for _, a := range t.Attr {
					if a.Name.Local == "n" {
						marker = a.Value
					}
					if a.Name.Local == "type" {
						noteType = a.Value
					}
				}
				if marker == "" {
					marker = strconv.Itoa(footnoteNum)
					footnoteNum++
				}

				// capture inner raw XML so we can extract references if present
				var inner bytes.Buffer
				enc := xml.NewEncoder(&inner)
				depth := 1
				for depth > 0 {
					nt, err := dec.Token()
					if err != nil {
						break
					}
					switch nt.(type) {
					case xml.StartElement:
						depth++
					case xml.EndElement:
						depth--
					}
					if depth > 0 {
						enc.EncodeToken(nt)
					}
				}
				enc.Flush()
				innerRaw := inner.String()
				clean := strings.TrimSpace(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(innerRaw, ""))

				if noteType == "crossReference" || strings.Contains(innerRaw, "osisRef=") {
					refs := []string{}
					for _, m := range refTagPattern.FindAllStringSubmatch(innerRaw, -1) {
						if len(m) > 1 {
							refs = append(refs, m[1])
						}
					}
					result.CrossRefs = append(result.CrossRefs, CrossReference{Marker: marker, References: refs, Text: clean})
					out.WriteString("__REFNOTE_" + marker + "__")
				} else {
					result.Footnotes = append(result.Footnotes, Footnote{Marker: marker, Text: clean, Type: noteType})
					out.WriteString("__NOTE_" + marker + "__")
				}
				continue
			}

			if strings.EqualFold(name, "reference") {
				marker := ""
				refs := []string{}
				for _, a := range t.Attr {
					if a.Name.Local == "osisRef" {
						refs = append(refs, a.Value)
					}
					if a.Name.Local == "n" {
						marker = a.Value
					}
				}
				if marker == "" {
					marker = string(rune('A') + rune(crossRefCounter%26))
					crossRefCounter++
				}
				result.CrossRefs = append(result.CrossRefs, CrossReference{Marker: marker, References: refs, Text: ""})
				// consume inner but ignore its output
				depth := 1
				for depth > 0 {
					nt, err := dec.Token()
					if err != nil {
						break
					}
					switch nt.(type) {
					case xml.StartElement:
						depth++
					case xml.EndElement:
						depth--
					}
				}
				out.WriteString("__REF_" + marker + "__")
				continue
			}

			// for other start elements we don't emit tags — text nodes will be handled below

		case xml.CharData:
			out.Write([]byte(t))
		case xml.EndElement:
			// Some EndElement tokens correspond to self-closing <q sID/> or <q eID/> markers
			if strings.EqualFold(t.Name.Local, "q") {
				if skipQEnd > 0 {
					skipQEnd--
					continue
				}
				// regular closing </q>
				if len(redWhoStack) > 0 {
					redWhoStack = redWhoStack[:len(redWhoStack)-1]
				}
				if redSpanOpen {
					out.WriteString(`</span>`)
					redSpanOpen = false
				}
				continue
			}
		default:
			// ignore other tokens
		}
	}

	pass1 := out.String()
	result.DebugPass1 = pass1

	// Second pass: replace placeholders with <sup> markers
	text := pass1
	noteRe := regexp.MustCompile(`__NOTE_([^_]+)__`)
	text = noteRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := noteRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		if filters.ShowFootnotes {
			return `<sup class="footnote">` + sub[1] + `</sup>` + "\u200B"
		}
		return ""
	})
	refRe := regexp.MustCompile(`__REF_([^_]+)__`)
	text = refRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := refRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		if filters.ShowScripref {
			return `<sup class="cross-ref">` + sub[1] + `</sup>` + "\u200B"
		}
		return ""
	})
	// also replace combined refnotes (if any)
	refnoteRe := regexp.MustCompile(`__REFNOTE_([^_]+)__`)
	text = refnoteRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := refnoteRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		if filters.ShowScripref {
			return `<sup class="cross-ref">` + sub[1] + `</sup>` + "\u200B"
		}
		return ""
	})

	// normalize whitespace
	text = strings.Join(strings.Fields(text), " ")
	result.Text = strings.TrimSpace(text)
	result.DebugPass2 = result.Text

	return result
}

// NOTE: lexicon lookup code has been removed. If you want to re-enable
// loading a hard-coded Strong's lexicon, restore the loader and lookup
// functions removed here.
