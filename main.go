package main

import (
	"bytes"
	"compress/zlib"
	"embed"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

//go:embed static
var staticFiles embed.FS

// Book represents a book in the Bible with its verse structure
type Book struct {
	Name           string
	OSIS           string
	Abbrev         string
	ChapterLengths []int // Number of verses per chapter
}

// KJV Canon - Standard Protestant Bible versification
var kjvOT = []Book{
	{"Genesis", "Gen", "Gen", []int{31, 25, 24, 26, 32, 22, 24, 22, 29, 32, 32, 20, 18, 24, 21, 16, 27, 33, 38, 18, 34, 24, 20, 67, 34, 35, 46, 22, 35, 43, 55, 32, 20, 31, 29, 43, 36, 30, 23, 23, 57, 38, 34, 34, 28, 34, 31, 22, 33, 26}},
	{"Exodus", "Exod", "Exod", []int{22, 25, 22, 31, 23, 30, 25, 32, 35, 29, 10, 51, 22, 31, 27, 36, 16, 27, 25, 26, 36, 31, 33, 18, 40, 37, 21, 43, 46, 38, 18, 35, 23, 35, 35, 38, 29, 31, 43, 38}},
	{"Leviticus", "Lev", "Lev", []int{17, 16, 17, 35, 19, 30, 38, 36, 24, 20, 47, 8, 59, 57, 33, 34, 16, 30, 37, 27, 24, 33, 44, 23, 55, 46, 34}},
	{"Numbers", "Num", "Num", []int{54, 34, 51, 49, 31, 27, 89, 26, 23, 36, 35, 16, 33, 45, 41, 50, 13, 32, 22, 29, 35, 41, 30, 25, 18, 65, 23, 31, 40, 16, 54, 42, 56, 29, 34, 13}},
	{"Deuteronomy", "Deut", "Deut", []int{46, 37, 29, 49, 33, 25, 26, 20, 29, 22, 32, 32, 18, 29, 23, 22, 20, 22, 21, 20, 23, 30, 25, 22, 19, 19, 26, 68, 29, 20, 30, 52, 29, 12}},
	{"Joshua", "Josh", "Josh", []int{18, 24, 17, 24, 15, 27, 26, 35, 27, 43, 23, 24, 33, 15, 63, 10, 18, 28, 51, 9, 45, 34, 16, 33}},
	{"Judges", "Judg", "Judg", []int{36, 23, 31, 24, 31, 40, 25, 35, 57, 18, 40, 15, 25, 20, 20, 31, 13, 31, 30, 48, 25}},
	{"Ruth", "Ruth", "Ruth", []int{22, 23, 18, 22}},
	{"1 Samuel", "1Sam", "1Sam", []int{28, 36, 21, 22, 12, 21, 17, 22, 27, 27, 15, 25, 23, 52, 35, 23, 58, 30, 24, 42, 15, 23, 29, 22, 44, 25, 12, 25, 11, 31, 13}},
	{"2 Samuel", "2Sam", "2Sam", []int{27, 32, 39, 12, 25, 23, 29, 18, 13, 19, 27, 31, 39, 33, 37, 23, 29, 33, 43, 26, 22, 51, 39, 25}},
	{"1 Kings", "1Kgs", "1Kgs", []int{53, 46, 28, 34, 18, 38, 51, 66, 28, 29, 43, 33, 34, 31, 34, 34, 24, 46, 21, 43, 29, 53}},
	{"2 Kings", "2Kgs", "2Kgs", []int{18, 25, 27, 44, 27, 33, 20, 29, 37, 36, 21, 21, 25, 29, 38, 20, 41, 37, 37, 21, 26, 20, 37, 20, 30}},
	{"1 Chronicles", "1Chr", "1Chr", []int{54, 55, 24, 43, 26, 81, 40, 40, 44, 14, 47, 40, 14, 17, 29, 43, 27, 17, 19, 8, 30, 19, 32, 31, 31, 32, 34, 21, 30}},
	{"2 Chronicles", "2Chr", "2Chr", []int{17, 18, 17, 22, 14, 42, 22, 18, 31, 19, 23, 16, 22, 15, 19, 14, 19, 34, 11, 37, 20, 12, 21, 27, 28, 23, 9, 27, 36, 27, 21, 33, 25, 33, 27, 23}},
	{"Ezra", "Ezra", "Ezra", []int{11, 70, 13, 24, 17, 22, 28, 36, 15, 44}},
	{"Nehemiah", "Neh", "Neh", []int{11, 20, 32, 23, 19, 19, 73, 18, 38, 39, 36, 47, 31}},
	{"Esther", "Esth", "Esth", []int{22, 23, 15, 17, 14, 14, 10, 17, 32, 3}},
	{"Job", "Job", "Job", []int{22, 13, 26, 21, 27, 30, 21, 22, 35, 22, 20, 25, 28, 22, 35, 22, 16, 21, 29, 29, 34, 30, 17, 25, 6, 14, 23, 28, 25, 31, 40, 22, 33, 37, 16, 33, 24, 41, 30, 24, 34, 17}},
	{"Psalms", "Ps", "Ps", []int{6, 12, 8, 8, 12, 10, 17, 9, 20, 18, 7, 8, 6, 7, 5, 11, 15, 50, 14, 9, 13, 31, 6, 10, 22, 12, 14, 9, 11, 12, 24, 11, 22, 22, 28, 12, 40, 22, 13, 17, 13, 11, 5, 26, 17, 11, 9, 14, 20, 23, 19, 9, 6, 7, 23, 13, 11, 11, 17, 12, 8, 12, 11, 10, 13, 20, 7, 35, 36, 5, 24, 20, 28, 23, 10, 12, 20, 72, 13, 19, 16, 8, 18, 12, 13, 17, 7, 18, 52, 17, 16, 15, 5, 23, 11, 13, 12, 9, 9, 5, 8, 28, 22, 35, 45, 48, 43, 13, 31, 7, 10, 10, 9, 8, 18, 19, 2, 29, 176, 7, 8, 9, 4, 8, 5, 6, 5, 6, 8, 8, 3, 18, 3, 3, 21, 26, 9, 8, 24, 13, 10, 7, 12, 15, 21, 10, 20, 14, 9, 6}},
	{"Proverbs", "Prov", "Prov", []int{33, 22, 35, 27, 23, 35, 27, 36, 18, 32, 31, 28, 25, 35, 33, 33, 28, 24, 29, 30, 31, 29, 35, 34, 28, 28, 27, 28, 27, 33, 31}},
	{"Ecclesiastes", "Eccl", "Eccl", []int{18, 26, 22, 16, 20, 12, 29, 17, 18, 20, 10, 14}},
	{"Song of Solomon", "Song", "Song", []int{17, 17, 11, 16, 16, 13, 13, 14}},
	{"Isaiah", "Isa", "Isa", []int{31, 22, 26, 6, 30, 13, 25, 22, 21, 34, 16, 6, 22, 32, 9, 14, 14, 7, 25, 6, 17, 25, 18, 23, 12, 21, 13, 29, 24, 33, 9, 20, 24, 17, 10, 22, 38, 22, 8, 31, 29, 25, 28, 28, 25, 13, 15, 22, 26, 11, 23, 15, 12, 17, 13, 12, 21, 14, 21, 22, 11, 12, 19, 12, 25, 24}},
	{"Jeremiah", "Jer", "Jer", []int{19, 37, 25, 31, 31, 30, 34, 22, 26, 25, 23, 17, 27, 22, 21, 21, 27, 23, 15, 18, 14, 30, 40, 10, 38, 24, 22, 17, 32, 24, 40, 44, 26, 22, 19, 32, 21, 28, 18, 16, 18, 22, 13, 30, 5, 28, 7, 47, 39, 46, 64, 34}},
	{"Lamentations", "Lam", "Lam", []int{22, 22, 66, 22, 22}},
	{"Ezekiel", "Ezek", "Ezek", []int{28, 10, 27, 17, 17, 14, 27, 18, 11, 22, 25, 28, 23, 23, 8, 63, 24, 32, 14, 49, 32, 31, 49, 27, 17, 21, 36, 26, 21, 26, 18, 32, 33, 31, 15, 38, 28, 23, 29, 49, 26, 20, 27, 31, 25, 24, 23, 35}},
	{"Daniel", "Dan", "Dan", []int{21, 49, 30, 37, 31, 28, 28, 27, 27, 21, 45, 13}},
	{"Hosea", "Hos", "Hos", []int{11, 23, 5, 19, 15, 11, 16, 14, 17, 15, 12, 14, 16, 9}},
	{"Joel", "Joel", "Joel", []int{20, 32, 21}},
	{"Amos", "Amos", "Amos", []int{15, 16, 15, 13, 27, 14, 17, 14, 15}},
	{"Obadiah", "Obad", "Obad", []int{21}},
	{"Jonah", "Jonah", "Jonah", []int{17, 10, 10, 11}},
	{"Micah", "Mic", "Mic", []int{16, 13, 12, 13, 15, 16, 20}},
	{"Nahum", "Nah", "Nah", []int{15, 13, 19}},
	{"Habakkuk", "Hab", "Hab", []int{17, 20, 19}},
	{"Zephaniah", "Zeph", "Zeph", []int{18, 15, 20}},
	{"Haggai", "Hag", "Hag", []int{15, 23}},
	{"Zechariah", "Zech", "Zech", []int{21, 13, 10, 14, 11, 15, 14, 23, 17, 12, 17, 14, 9, 21}},
	{"Malachi", "Mal", "Mal", []int{14, 17, 18, 6}},
}

var kjvNT = []Book{
	{"Matthew", "Matt", "Matt", []int{25, 23, 17, 25, 48, 34, 29, 34, 38, 42, 30, 50, 58, 36, 39, 28, 27, 35, 30, 34, 46, 46, 39, 51, 46, 75, 66, 20}},
	{"Mark", "Mark", "Mark", []int{45, 28, 35, 41, 43, 56, 37, 38, 50, 52, 33, 44, 37, 72, 47, 20}},
	{"Luke", "Luke", "Luke", []int{80, 52, 38, 44, 39, 49, 50, 56, 62, 42, 54, 59, 35, 35, 32, 31, 37, 43, 48, 47, 38, 71, 56, 53}},
	{"John", "John", "John", []int{51, 25, 36, 54, 47, 71, 53, 59, 41, 42, 57, 50, 38, 31, 27, 33, 26, 40, 42, 31, 25}},
	{"Acts", "Acts", "Acts", []int{26, 47, 26, 37, 42, 15, 60, 40, 43, 48, 30, 25, 52, 28, 41, 40, 34, 28, 41, 38, 40, 30, 35, 27, 27, 32, 44, 31}},
	{"Romans", "Rom", "Rom", []int{32, 29, 31, 25, 21, 23, 25, 39, 33, 21, 36, 21, 14, 23, 33, 27}},
	{"1 Corinthians", "1Cor", "1Cor", []int{31, 16, 23, 21, 13, 20, 40, 13, 27, 33, 34, 31, 13, 40, 58, 24}},
	{"2 Corinthians", "2Cor", "2Cor", []int{24, 17, 18, 18, 21, 18, 16, 24, 15, 18, 33, 21, 14}},
	{"Galatians", "Gal", "Gal", []int{24, 21, 29, 31, 26, 18}},
	{"Ephesians", "Eph", "Eph", []int{23, 22, 21, 32, 33, 24}},
	{"Philippians", "Phil", "Phil", []int{30, 30, 21, 23}},
	{"Colossians", "Col", "Col", []int{29, 23, 25, 18}},
	{"1 Thessalonians", "1Thess", "1Thess", []int{10, 20, 13, 18, 28}},
	{"2 Thessalonians", "2Thess", "2Thess", []int{12, 17, 18}},
	{"1 Timothy", "1Tim", "1Tim", []int{20, 15, 16, 16, 25, 21}},
	{"2 Timothy", "2Tim", "2Tim", []int{18, 26, 17, 22}},
	{"Titus", "Titus", "Titus", []int{16, 15, 15}},
	{"Philemon", "Phlm", "Phlm", []int{25}},
	{"Hebrews", "Heb", "Heb", []int{14, 18, 19, 16, 14, 20, 28, 13, 28, 39, 40, 29, 25}},
	{"James", "Jas", "Jas", []int{27, 26, 18, 17, 20}},
	{"1 Peter", "1Pet", "1Pet", []int{25, 25, 22, 19, 14}},
	{"2 Peter", "2Pet", "2Pet", []int{21, 22, 18}},
	{"1 John", "1John", "1John", []int{10, 29, 24, 21, 21}},
	{"2 John", "2John", "2John", []int{13}},
	{"3 John", "3John", "3John", []int{14}},
	{"Jude", "Jude", "Jude", []int{25}},
	{"Revelation", "Rev", "Rev", []int{20, 29, 22, 11, 14, 17, 17, 13, 21, 11, 19, 17, 18, 20, 8, 21, 18, 24, 21, 15, 27, 21}},
}

// ZTextReader reads compressed Bible text from Sword module files (.bzv, .bzs, .bzz)
type ZTextReader struct {
	bzvFile *os.File
	bzsFile *os.File
	bzzFile *os.File
}

// NewZTextReader creates a new ZTextReader for the given base path
func NewZTextReader(basePath string) (*ZTextReader, error) {
	bzv, err := os.Open(basePath + ".bzv")
	if err != nil {
		return nil, err
	}
	bzs, err := os.Open(basePath + ".bzs")
	if err != nil {
		if closeErr := bzv.Close(); closeErr != nil {
			log.Printf("Failed to close bzv file: %v", closeErr)
		}
		return nil, err
	}
	bzz, err := os.Open(basePath + ".bzz")
	if err != nil {
		if closeErr := bzv.Close(); closeErr != nil {
			log.Printf("Failed to close bzv file: %v", closeErr)
		}
		if closeErr := bzs.Close(); closeErr != nil {
			log.Printf("Failed to close bzs file: %v", closeErr)
		}
		return nil, err
	}
	return &ZTextReader{bzvFile: bzv, bzsFile: bzs, bzzFile: bzz}, nil
}

// Close closes all open file handles
func (z *ZTextReader) Close() error {
	var errs []error
	if err := z.bzvFile.Close(); err != nil {
		errs = append(errs, fmt.Errorf("closing bzv file: %w", err))
	}
	if err := z.bzsFile.Close(); err != nil {
		errs = append(errs, fmt.Errorf("closing bzs file: %w", err))
	}
	if err := z.bzzFile.Close(); err != nil {
		errs = append(errs, fmt.Errorf("closing bzz file: %w", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing files: %v", errs)
	}
	return nil
}

// ReadVerse reads and decompresses a verse at the given index
func (z *ZTextReader) ReadVerse(index uint32) (string, error) {
	// Read 10-byte verse record from .bzv
	if _, err := z.bzvFile.Seek(int64(index*10), 0); err != nil {
		return "", fmt.Errorf("seeking bzv file: %w", err)
	}
	var bufNum, start uint32
	var length uint16
	if err := binary.Read(z.bzvFile, binary.LittleEndian, &bufNum); err != nil {
		return "", fmt.Errorf("reading bufNum: %w", err)
	}
	if err := binary.Read(z.bzvFile, binary.LittleEndian, &start); err != nil {
		return "", fmt.Errorf("reading start: %w", err)
	}
	if err := binary.Read(z.bzvFile, binary.LittleEndian, &length); err != nil {
		return "", fmt.Errorf("reading length: %w", err)
	}

	// Read 12-byte buffer record from .bzs
	if _, err := z.bzsFile.Seek(int64(bufNum*12), 0); err != nil {
		return "", fmt.Errorf("seeking bzs file: %w", err)
	}
	var offset, compSize, uncompSize uint32
	if err := binary.Read(z.bzsFile, binary.LittleEndian, &offset); err != nil {
		return "", fmt.Errorf("reading offset: %w", err)
	}
	if err := binary.Read(z.bzsFile, binary.LittleEndian, &compSize); err != nil {
		return "", fmt.Errorf("reading compSize: %w", err)
	}
	if err := binary.Read(z.bzsFile, binary.LittleEndian, &uncompSize); err != nil {
		return "", fmt.Errorf("reading uncompSize: %w", err)
	}

	// Read compressed data from .bzz
	if _, err := z.bzzFile.Seek(int64(offset), 0); err != nil {
		return "", fmt.Errorf("seeking bzz file: %w", err)
	}
	compData := make([]byte, compSize)
	if _, err := io.ReadFull(z.bzzFile, compData); err != nil {
		return "", fmt.Errorf("reading compressed data: %w", err)
	}

	// Decompress
	r, err := zlib.NewReader(bytes.NewReader(compData))
	if err != nil {
		return "", fmt.Errorf("creating zlib reader: %w", err)
	}
	uncompData := make([]byte, uncompSize)
	if _, err := io.ReadFull(r, uncompData); err != nil {
		return "", fmt.Errorf("reading uncompressed data: %w", err)
	}
	if err := r.Close(); err != nil {
		return "", fmt.Errorf("closing zlib reader: %w", err)
	}

	// Extract verse text
	verseText := string(uncompData[start : start+uint32(length)])
	return strings.TrimSpace(verseText), nil
}

// BibleStructure calculates verse indices based on KJV canon
type BibleStructure struct {
	Books      []Book
	BookOffset map[string]uint32 // book name -> starting index in testament
}

// NewBibleStructure creates a new BibleStructure with calculated verse offsets
func NewBibleStructure(books []Book) *BibleStructure {
	bs := &BibleStructure{
		Books:      books,
		BookOffset: make(map[string]uint32),
	}

	// Calculate book offsets
	// Start at 2 (testament has 2 introductory entries)
	idx := uint32(2)
	for _, book := range books {
		bs.BookOffset[book.Name] = idx
		// Each book has: 1 book heading + sum of (1 chapter heading + verses per chapter)
		idx++ // book heading
		for _, chapterLen := range book.ChapterLengths {
			idx += 1 + uint32(chapterLen) // chapter heading + verses
		}
	}

	return bs
}

// GetVerseIndex calculates the index for a specific verse within the testament
func (bs *BibleStructure) GetVerseIndex(bookName string, chapter, verse int) (uint32, error) {
	// Find the book
	var book *Book
	for i := range bs.Books {
		if bs.Books[i].Name == bookName || bs.Books[i].OSIS == bookName {
			book = &bs.Books[i]
			break
		}
	}
	if book == nil {
		return 0, fmt.Errorf("book not found: %s", bookName)
	}

	if chapter < 1 || chapter > len(book.ChapterLengths) {
		return 0, fmt.Errorf("invalid chapter %d for %s", chapter, bookName)
	}
	if verse < 1 || verse > book.ChapterLengths[chapter-1] {
		return 0, fmt.Errorf("invalid verse %d for %s %d", verse, bookName, chapter)
	}

	// Calculate index: book_offset + book_heading(1) + chapter_offset + chapter_heading(1) + verse
	idx := bs.BookOffset[bookName]
	idx++ // book heading

	// Add verses from previous chapters
	for i := 0; i < chapter-1; i++ {
		idx += 1 + uint32(book.ChapterLengths[i]) // chapter heading + verses
	}
	idx++                    // current chapter heading
	idx += uint32(verse - 1) // verse (0-indexed within chapter)

	return idx, nil
}

// Translation represents a Bible translation with its readers and structure
type Translation struct {
	Name        string
	FullName    string
	Description string
	OTReader    *ZTextReader
	NTReader    *ZTextReader
	OTStructure *BibleStructure
	NTStructure *BibleStructure
	HasOT       bool
	HasNT       bool
}

var translations map[string]*Translation
var translationNames []string
var tagPattern = regexp.MustCompile(`<[^>]+>`)
var sectionTitlePattern = regexp.MustCompile(`<title[^>]*(?:subType="x-preverse"|type="x-s")[^>]*>([^<]+)</title>`)
var descriptionTitlePattern = regexp.MustCompile(`<title[^>]*type="x-description"[^>]*>([^<]+)</title>`)
var parallelTitlePattern = regexp.MustCompile(`<title[^>]*type="parallel"[^>]*>(.*?)</title>`)
var crossRefPattern = regexp.MustCompile(`<note[^>]*type="crossReference"[^>]*>(.*?)</note>`)
var crossRefWithMarkerPattern = regexp.MustCompile(`<note\s+n="([^"]+)"[^>]*type="crossReference"[^>]*>(.*?)</note>`)
var explanationPattern = regexp.MustCompile(`<note[^>]*(?:type="explanation"|placement="foot")[^>]*>(.*?)</note>`)
var explanationWithMarkerPattern = regexp.MustCompile(`<note\s+n="([^"]+)"[^>]*type="explanation"[^>]*>(.*?)</note>`)
var genericNotePattern = regexp.MustCompile(`<note(?:\s+[^>]*)?>(.*?)</note>`)
var studyNotePattern = regexp.MustCompile(`<note[^>]*type="study"[^>]*>(.*?)</note>`)
var studyNoteWithMarkerPattern = regexp.MustCompile(`<note\s+n="([^"]+)"[^>]*type="study"[^>]*>(.*?)</note>`)
var catchWordPattern = regexp.MustCompile(`<catchWord>([^<]+)</catchWord>`)
var referencePattern = regexp.MustCompile(`<reference[^>]*>([^<]+)</reference>`)
var transChangePattern = regexp.MustCompile(`<transChange[^>]*type="added"[^>]*>([^<]*)</transChange>`)
var milestonePattern = regexp.MustCompile(`<milestone[^>]*/>`)
var hiItalicPattern = regexp.MustCompile(`<hi type="italic">([^<]+)</hi>`)
var hiBoldPattern = regexp.MustCompile(`<hi type="bold">([^<]+)</hi>`)
var lineBreakPattern = regexp.MustCompile(`<lb/>`)

func cleanText(text string) string {
	// Convert italic formatting to HTML tags
	// LEB/NASB italic text: <hi type="italic">text</hi> -> <i>text</i>
	cleaned := hiItalicPattern.ReplaceAllString(text, "<i>$1</i>")

	// Remove bold tags (used in EMTV translator notes)
	cleaned = hiBoldPattern.ReplaceAllString(cleaned, "$1")

	// Replace line breaks with spaces
	cleaned = lineBreakPattern.ReplaceAllString(cleaned, " ")

	// Temporarily replace our HTML tags with placeholders
	cleaned = strings.ReplaceAll(cleaned, "<i>", "⟪ITALIC_START⟫")
	cleaned = strings.ReplaceAll(cleaned, "</i>", "⟪ITALIC_END⟫")
	cleaned = strings.ReplaceAll(cleaned, "<sup>", "⟪SUP_START⟫")
	cleaned = strings.ReplaceAll(cleaned, "</sup>", "⟪SUP_END⟫")

	// Remove all XML tags
	cleaned = tagPattern.ReplaceAllString(cleaned, "")

	// Restore HTML tags
	cleaned = strings.ReplaceAll(cleaned, "⟪ITALIC_START⟫", "<i>")
	cleaned = strings.ReplaceAll(cleaned, "⟪ITALIC_END⟫", "</i>")
	cleaned = strings.ReplaceAll(cleaned, "⟪SUP_START⟫", "<sup>")
	cleaned = strings.ReplaceAll(cleaned, "⟪SUP_END⟫", "</sup>")

	// Normalize whitespace
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.TrimSpace(cleaned)
}

func parseVerseData(text string) map[string]interface{} {
	result := make(map[string]interface{})

	// Extract section title (appears before verse) - works for both NASB and LEB
	if match := sectionTitlePattern.FindStringSubmatch(text); len(match) > 1 {
		result["sectionTitle"] = strings.TrimSpace(match[1])
	}

	// Extract chapter description (AKJV) - appears at beginning of chapter
	if match := descriptionTitlePattern.FindStringSubmatch(text); len(match) > 1 {
		result["introNote"] = strings.TrimSpace(match[1])
	}

	// Extract cross-references with markers (NASB)
	type NoteWithMarker struct {
		Marker string `json:"marker,omitempty"`
		Text   string `json:"text"`
	}

	crossRefs := []NoteWithMarker{}

	// BSB parallel references - extract and add to cross-references
	for _, match := range parallelTitlePattern.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			noteContent := match[1]
			refs := referencePattern.FindAllStringSubmatch(noteContent, -1)
			refList := []string{}
			for _, ref := range refs {
				if len(ref) > 1 {
					refList = append(refList, strings.TrimSpace(ref[1]))
				}
			}
			if len(refList) > 0 {
				crossRefs = append(crossRefs, NoteWithMarker{Text: "See also: " + strings.Join(refList, "; ")})
			}
		}
	}

	// First try with markers (NASB)
	for _, match := range crossRefWithMarkerPattern.FindAllStringSubmatch(text, -1) {
		if len(match) > 2 {
			marker := match[1]
			noteContent := match[2]
			refs := referencePattern.FindAllStringSubmatch(noteContent, -1)
			refList := []string{}
			for _, ref := range refs {
				if len(ref) > 1 {
					refList = append(refList, strings.TrimSpace(ref[1]))
				}
			}
			if len(refList) > 0 {
				crossRefs = append(crossRefs, NoteWithMarker{Marker: marker, Text: strings.Join(refList, "; ")})
			}
		}
	}
	// Fallback for notes without markers (LEB)
	if len(crossRefs) == 0 {
		for _, match := range crossRefPattern.FindAllStringSubmatch(text, -1) {
			if len(match) > 1 {
				noteContent := match[1]
				refs := referencePattern.FindAllStringSubmatch(noteContent, -1)
				refList := []string{}
				for _, ref := range refs {
					if len(ref) > 1 {
						refList = append(refList, strings.TrimSpace(ref[1]))
					}
				}
				if len(refList) > 0 {
					crossRefs = append(crossRefs, NoteWithMarker{Text: strings.Join(refList, "; ")})
				}
			}
		}
	}
	if len(crossRefs) > 0 {
		result["crossReferences"] = crossRefs
	}

	// Extract study notes (KJV marginal notes and alternative readings)
	// These use catchWord tags to indicate where the marker should appear
	type StudyNoteWithCatchWord struct {
		Marker    string
		Text      string
		CatchWord string
	}
	studyNotesWithCatch := []StudyNoteWithCatchWord{}

	// Extract study notes and their catch words
	letterIdx := 0
	for _, match := range studyNotePattern.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			noteContent := match[1]
			// Extract catch word
			catchWord := ""
			if cwMatch := catchWordPattern.FindStringSubmatch(noteContent); len(cwMatch) > 1 {
				catchWord = cwMatch[1]
			}
			// Remove all tags to get clean note text
			noteText := tagPattern.ReplaceAllString(noteContent, "")
			noteText = strings.TrimSpace(noteText)
			if noteText != "" {
				// Assign letter marker (A, B, C, etc.)
				marker := string(rune('A' + letterIdx))
				letterIdx++
				studyNotesWithCatch = append(studyNotesWithCatch, StudyNoteWithCatchWord{
					Marker:    marker,
					Text:      noteText,
					CatchWord: catchWord,
				})
			}
		}
	}

	// Convert to output format for JSON
	studyNotes := []NoteWithMarker{}
	for _, note := range studyNotesWithCatch {
		studyNotes = append(studyNotes, NoteWithMarker{Marker: note.Marker, Text: note.Text})
	}
	if len(studyNotes) > 0 {
		result["studyNotes"] = studyNotes
	}

	// Extract explanatory notes with markers (both NASB and LEB)
	notes := []NoteWithMarker{}
	// First try with markers (NASB)
	for _, match := range explanationWithMarkerPattern.FindAllStringSubmatch(text, -1) {
		if len(match) > 2 {
			marker := match[1]
			noteText := tagPattern.ReplaceAllString(match[2], "")
			noteText = strings.TrimSpace(noteText)
			if noteText != "" {
				notes = append(notes, NoteWithMarker{Marker: marker, Text: noteText})
			}
		}
	}
	// Fallback for notes without markers (LEB) - assign letters
	if len(notes) == 0 {
		letterIdx := 0
		for _, match := range explanationPattern.FindAllStringSubmatch(text, -1) {
			if len(match) > 1 {
				noteText := tagPattern.ReplaceAllString(match[1], "")
				noteText = strings.TrimSpace(noteText)
				if noteText != "" {
					marker := string(rune('a' + letterIdx))
					notes = append(notes, NoteWithMarker{Marker: marker, Text: noteText})
					letterIdx++
				}
			}
		}
	}
	if len(notes) > 0 {
		result["notes"] = notes
	}

	// Extract generic notes (EMTV) - these don't have type attributes
	// Only process if no other notes were found to avoid duplicates
	if len(notes) == 0 {
		letterIdx := 0
		for _, match := range genericNotePattern.FindAllStringSubmatch(text, -1) {
			if len(match) > 1 {
				noteContent := match[1]
				// Check if this is a very long note (like EMTV translator's message)
				if len(noteContent) > 500 {
					// Extract as introductory note
					// First convert reference tags to plain text
					noteText := referencePattern.ReplaceAllString(noteContent, "$1")
					noteText = hiBoldPattern.ReplaceAllString(noteText, "")
					noteText = lineBreakPattern.ReplaceAllString(noteText, "\n")
					noteText = tagPattern.ReplaceAllString(noteText, "")
					noteText = strings.TrimSpace(noteText)
					if noteText != "" {
						result["introNote"] = noteText
					}
					continue
				}
				// Convert reference tags to plain text before removing other tags
				noteText := referencePattern.ReplaceAllString(noteContent, "$1")
				noteText = hiBoldPattern.ReplaceAllString(noteText, "")
				noteText = lineBreakPattern.ReplaceAllString(noteText, " ")
				noteText = tagPattern.ReplaceAllString(noteText, "")
				noteText = strings.TrimSpace(noteText)
				if noteText != "" {
					marker := string(rune('a' + letterIdx))
					notes = append(notes, NoteWithMarker{Marker: marker, Text: noteText})
					letterIdx++
				}
			}
		}
		if len(notes) > 0 {
			result["notes"] = notes
		}
	}

	// Remove section titles from the main text
	cleanedText := text
	cleanedText = sectionTitlePattern.ReplaceAllString(cleanedText, "")
	cleanedText = descriptionTitlePattern.ReplaceAllString(cleanedText, "")
	cleanedText = parallelTitlePattern.ReplaceAllString(cleanedText, "")

	// Replace catch words in study notes with markers
	// We need to do this before removing the study notes themselves
	for _, studyNote := range studyNotesWithCatch {
		if studyNote.CatchWord != "" {
			catchWord := studyNote.CatchWord

			// Check if catch word ends with ellipsis (…) indicating partial match
			isPartial := strings.HasSuffix(catchWord, "…") || strings.HasSuffix(catchWord, "...")
			if isPartial {
				// Strip ellipsis for partial matching
				catchWord = strings.TrimSuffix(catchWord, "…")
				catchWord = strings.TrimSuffix(catchWord, "...")
				// Match the prefix of a word
				catchWordPattern := regexp.MustCompile(`(>|\s|^)(` + regexp.QuoteMeta(catchWord) + `[^<]*)`)
				cleanedText = catchWordPattern.ReplaceAllString(cleanedText, `${1}${2}<sup>[`+studyNote.Marker+`]</sup>`)
			} else {
				// Exact match for whole words
				catchWordPattern := regexp.MustCompile(`(>|\s|^)(` + regexp.QuoteMeta(catchWord) + `)(</w>|<|\s|[.,;:]|$)`)
				cleanedText = catchWordPattern.ReplaceAllString(cleanedText, `${1}${2}<sup>[`+studyNote.Marker+`]</sup>${3}`)
			}
		}
	}

	// Remove study notes (KJV marginal notes) completely
	cleanedText = studyNoteWithMarkerPattern.ReplaceAllString(cleanedText, "")
	cleanedText = studyNotePattern.ReplaceAllString(cleanedText, "")

	// Replace cross-references with markers
	cleanedText = crossRefWithMarkerPattern.ReplaceAllString(cleanedText, "<sup>[$1]</sup>")
	cleanedText = crossRefPattern.ReplaceAllString(cleanedText, "") // Remove unmarked ones

	// Replace explanatory notes with markers
	if len(notes) > 0 {
		// For NASB with markers
		cleanedText = explanationWithMarkerPattern.ReplaceAllString(cleanedText, "<sup>[$1]</sup>")
		// For LEB/EMTV without markers - use sequential letters
		letterIdx := 0
		// First try specific typed notes
		cleanedText = explanationPattern.ReplaceAllStringFunc(cleanedText, func(match string) string {
			if letterIdx < len(notes) {
				marker := notes[letterIdx].Marker
				letterIdx++
				return "<sup>[" + marker + "]</sup>"
			}
			return ""
		})
		// Then generic notes (EMTV)
		cleanedText = genericNotePattern.ReplaceAllStringFunc(cleanedText, func(match string) string {
			// Skip very long notes (like translator's message)
			if len(match) > 500 {
				return ""
			}
			if letterIdx < len(notes) {
				marker := notes[letterIdx].Marker
				letterIdx++
				return "<sup>[" + marker + "]</sup>"
			}
			return ""
		})
	} else {
		cleanedText = explanationPattern.ReplaceAllString(cleanedText, "")
		// Remove generic notes without markers
		cleanedText = genericNotePattern.ReplaceAllStringFunc(cleanedText, func(match string) string {
			// Always remove long notes (translator's message)
			return ""
		})
	}

	// Convert LEB idioms to italic tags BEFORE removing milestones
	// Pattern: <milestone type="x-idiom-start"/>⌞text⌟<milestone type="x-idiom-end"/>
	// Unicode: ⌞ = U+231E, ⌟ = U+231F
	idiomWithMilestones := regexp.MustCompile(`<milestone type="x-idiom-start"/>⌞([^⌟]+)⌟<milestone type="x-idiom-end"/>`)
	cleanedText = idiomWithMilestones.ReplaceAllString(cleanedText, "<i>$1</i>")

	// Remove remaining milestones (LEB specific)
	cleanedText = milestonePattern.ReplaceAllString(cleanedText, "")

	// Remove x-preverse and other divs
	cleanedText = regexp.MustCompile(`<div[^>]*subType="x-preverse"[^>]*>.*?</div>`).ReplaceAllString(cleanedText, "")
	cleanedText = regexp.MustCompile(`<div[^>]*type="x-milestone"[^>]*/>`).ReplaceAllString(cleanedText, "")
	cleanedText = regexp.MustCompile(`<div[^>]*type="x-milestone"[^>]*>`).ReplaceAllString(cleanedText, "")
	cleanedText = regexp.MustCompile(`<div[^>]*type="introduction"[^>]*/?>`).ReplaceAllString(cleanedText, "")
	cleanedText = regexp.MustCompile(`<div[^>]*(?:sID|eID)="[^"]*"[^>]*/?>`).ReplaceAllString(cleanedText, "")
	cleanedText = regexp.MustCompile(`<div[^>]*type="x-p"[^>]*/?>`).ReplaceAllString(cleanedText, "")
	cleanedText = regexp.MustCompile(`</div>`).ReplaceAllString(cleanedText, "")

	// Handle transChange tags (LEB) - keep the added text but mark it
	cleanedText = transChangePattern.ReplaceAllString(cleanedText, "$1")

	// Remove chapter tags
	cleanedText = regexp.MustCompile(`<chapter[^>]*>.*?</chapter>`).ReplaceAllString(cleanedText, "")
	cleanedText = regexp.MustCompile(`<chapter[^>]*/>`).ReplaceAllString(cleanedText, "")

	// Clean main text (remove remaining XML tags)
	result["text"] = cleanText(cleanedText)

	return result
}

func loadTranslation(name, path, fullName, description, testaments string) error {
	log.Printf("Loading translation: %s from %s", name, path)

	// Default to "both" if not specified
	if testaments == "" {
		testaments = "both"
	}

	var otReader *ZTextReader
	var ntReader *ZTextReader
	var err error

	// Load OT if needed
	hasOT := testaments == "both" || testaments == "ot"
	if hasOT {
		otReader, err = NewZTextReader(path + "/ot")
		if err != nil {
			return fmt.Errorf("loading OT: %w", err)
		}
	}

	// Load NT if needed
	hasNT := testaments == "both" || testaments == "nt"
	if hasNT {
		ntReader, err = NewZTextReader(path + "/nt")
		if err != nil {
			if otReader != nil {
				if closeErr := otReader.Close(); closeErr != nil {
					log.Printf("Failed to close OT reader: %v", closeErr)
				}
			}
			return fmt.Errorf("loading NT: %w", err)
		}
	}

	translations[name] = &Translation{
		Name:        name,
		FullName:    fullName,
		Description: description,
		OTReader:    otReader,
		NTReader:    ntReader,
		OTStructure: NewBibleStructure(kjvOT),
		NTStructure: NewBibleStructure(kjvNT),
		HasOT:       hasOT,
		HasNT:       hasNT,
	}

	// Calculate verse counts based on what's configured
	otTotal := uint32(0)
	if hasOT {
		for _, book := range kjvOT {
			for _, chapterLen := range book.ChapterLengths {
				otTotal += uint32(chapterLen)
			}
		}
	}
	ntTotal := uint32(0)
	if hasNT {
		for _, book := range kjvNT {
			for _, chapterLen := range book.ChapterLengths {
				ntTotal += uint32(chapterLen)
			}
		}
	}
	log.Printf("Loaded %s: %d OT verses, %d NT verses (Total: %d)", name, otTotal, ntTotal, otTotal+ntTotal)

	return nil
}

func getChapter(translation, bookName string, chapter int) ([]map[string]interface{}, error) {
	trans, ok := translations[translation]
	if !ok {
		return nil, fmt.Errorf("translation not found: %s", translation)
	}

	// Find the book
	var book *Book
	var structure *BibleStructure
	var reader *ZTextReader
	for i := range kjvOT {
		if kjvOT[i].Name == bookName || kjvOT[i].OSIS == bookName {
			book = &kjvOT[i]
			structure = trans.OTStructure
			reader = trans.OTReader
			break
		}
	}
	if book == nil {
		for i := range kjvNT {
			if kjvNT[i].Name == bookName || kjvNT[i].OSIS == bookName {
				book = &kjvNT[i]
				structure = trans.NTStructure
				reader = trans.NTReader
				break
			}
		}
	}
	if book == nil {
		return nil, fmt.Errorf("book not found: %s", bookName)
	}

	// Verify the reader exists for this testament
	if reader == nil {
		testament := "Old"
		if structure == trans.NTStructure {
			testament = "New"
		}
		return nil, fmt.Errorf("%s Testament not available in translation %s", testament, translation)
	}

	if chapter < 1 || chapter > len(book.ChapterLengths) {
		return nil, fmt.Errorf("invalid chapter %d for %s", chapter, bookName)
	}

	verseCount := book.ChapterLengths[chapter-1]
	verses := make([]map[string]interface{}, verseCount)

	for v := 1; v <= verseCount; v++ {
		idx, _ := structure.GetVerseIndex(bookName, chapter, v)
		rawText, err := reader.ReadVerse(idx)
		if err != nil {
			rawText = ""
		}
		verseData := parseVerseData(rawText)
		verseData["verse"] = v
		verses[v-1] = verseData
	}

	return verses, nil
}

// HTTP Handlers
func handleTranslations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Return translation info with full names and descriptions
	translationInfo := make([]map[string]string, 0, len(translationNames))
	for _, name := range translationNames {
		if trans, ok := translations[name]; ok {
			translationInfo = append(translationInfo, map[string]string{
				"name":        trans.Name,
				"fullName":    trans.FullName,
				"description": trans.Description,
			})
		}
	}
	if err := json.NewEncoder(w).Encode(translationInfo); err != nil {
		log.Printf("Failed to encode translation info: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func handleBooks(w http.ResponseWriter, r *http.Request) {
	translationName := r.URL.Query().Get("translation")

	// Default to first translation if not specified
	if translationName == "" && len(translationNames) > 0 {
		translationName = translationNames[0]
	}

	// Get the translation to check which testaments are available
	trans, ok := translations[translationName]
	if !ok {
		http.Error(w, "Translation not found", http.StatusBadRequest)
		return
	}

	books := make([]map[string]interface{}, 0, len(kjvOT)+len(kjvNT))

	// Only include OT books if translation has OT
	if trans.HasOT {
		for _, book := range kjvOT {
			books = append(books, map[string]interface{}{
				"name":         book.Name,
				"chapterCount": len(book.ChapterLengths),
			})
		}
	}

	// Only include NT books if translation has NT
	if trans.HasNT {
		for _, book := range kjvNT {
			books = append(books, map[string]interface{}{
				"name":         book.Name,
				"chapterCount": len(book.ChapterLengths),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(books); err != nil {
		log.Printf("Failed to encode books: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func handleChapter(w http.ResponseWriter, r *http.Request) {
	book := r.URL.Query().Get("book")
	chapterStr := r.URL.Query().Get("chapter")
	translation := r.URL.Query().Get("translation")

	if translation == "" {
		translation = translationNames[0]
	}

	chapter, err := strconv.Atoi(chapterStr)
	if err != nil {
		http.Error(w, "Invalid chapter number", http.StatusBadRequest)
		return
	}

	verses, err := getChapter(translation, book, chapter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := map[string]interface{}{
		"book":    book,
		"chapter": chapter,
		"verses":  verses,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Parse command-line flags
	debugTranslation := flag.String("debug", "", "Translation to debug (e.g., kjv)")
	debugBook := flag.String("book", "", "Book name for debug mode (e.g., Genesis)")
	debugChapter := flag.Int("chapter", 0, "Chapter number for debug mode")
	debugVerse := flag.Int("verse", 0, "Verse number for debug mode")
	flag.Parse()

	// Initialize translations
	translations = make(map[string]*Translation)

	// Load configuration
	config, err := LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Ensure translations exist, download if missing
	if err := EnsureTranslationsExist(config); err != nil {
		log.Printf("Warning: Failed to download translations: %v", err)
		log.Println("Attempting to load existing translations...")
	}

	// Load translations from config
	for _, tc := range config.Translations {
		translationPath := filepath.Join("translations", tc.Name)
		if err := loadTranslation(tc.Name, translationPath, tc.FullName, tc.Description, tc.Testaments); err != nil {
			log.Printf("Warning: Could not load %s: %v", tc.Name, err)
		} else {
			translationNames = append(translationNames, tc.Name)
		}
	}

	if len(translations) == 0 {
		log.Fatal("No translations could be loaded")
	}

	// Debug mode: print raw verse text
	if *debugTranslation != "" {
		if *debugBook == "" || *debugChapter == 0 || *debugVerse == 0 {
			log.Fatal("Debug mode requires --debug, --book, --chapter, and --verse flags")
		}

		translation, ok := translations[*debugTranslation]
		if !ok {
			log.Fatalf("Translation '%s' not found. Available: %v", *debugTranslation, translationNames)
		}

		// Find the book
		var book *Book
		var structure *BibleStructure
		var reader *ZTextReader
		for i := range kjvOT {
			if strings.EqualFold(kjvOT[i].Name, *debugBook) || strings.EqualFold(kjvOT[i].OSIS, *debugBook) {
				book = &kjvOT[i]
				structure = translation.OTStructure
				reader = translation.OTReader
				break
			}
		}
		if book == nil {
			for i := range kjvNT {
				if strings.EqualFold(kjvNT[i].Name, *debugBook) || strings.EqualFold(kjvNT[i].OSIS, *debugBook) {
					book = &kjvNT[i]
					structure = translation.NTStructure
					reader = translation.NTReader
					break
				}
			}
		}
		if book == nil {
			log.Fatalf("Book '%s' not found", *debugBook)
		}

		if *debugChapter < 1 || *debugChapter > len(book.ChapterLengths) {
			log.Fatalf("Chapter %d out of range for book '%s' (1-%d)", *debugChapter, book.Name, len(book.ChapterLengths))
		}

		verseCount := book.ChapterLengths[*debugChapter-1]
		if *debugVerse < 1 || *debugVerse > verseCount {
			log.Fatalf("Verse %d out of range for chapter %d (1-%d)", *debugVerse, *debugChapter, verseCount)
		}

		idx, err := structure.GetVerseIndex(book.Name, *debugChapter, *debugVerse)
		if err != nil {
			log.Fatalf("Failed to get verse index: %v", err)
		}

		rawText, err := reader.ReadVerse(idx)
		if err != nil {
			log.Fatalf("Failed to read verse: %v", err)
		}

		fmt.Printf("Translation: %s\n", *debugTranslation)
		fmt.Printf("Book: %s\n", book.Name)
		fmt.Printf("Chapter: %d\n", *debugChapter)
		fmt.Printf("Verse: %d\n", *debugVerse)
		fmt.Printf("\nRaw verse text (including OSIS formatting):\n")
		fmt.Printf("%s\n", rawText)
		return
	} // Setup HTTP routes
	http.HandleFunc("/api/translations", handleTranslations)
	http.HandleFunc("/api/books", handleBooks)
	http.HandleFunc("/api/chapter", handleChapter)

	// Serve static files from embedded filesystem at root
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(staticFS)))

	log.Println("Server starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
