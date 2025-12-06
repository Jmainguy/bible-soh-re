// State management
let bibleBooks = [];
let currentBook = null;
let currentChapter = 1;
let maxChapter = 1;
let showReferences = true; // Toggle for showing/hiding references
let selectedTranslation = ''; // Currently selected translation
let translationInfo = {}; // Translation metadata (fullName, description)
let sidebarVisible = true; // Sidebar visibility state

// DOM Elements
const bookList = document.getElementById('bookList');
const chapterTitle = document.getElementById('chapterTitle');
const versesContainer = document.getElementById('versesContainer');
const prevChapterBtn = document.getElementById('prevChapter');
const nextChapterBtn = document.getElementById('nextChapter');
const chapterSelector = document.getElementById('chapterSelector');
const chapterSelect = document.getElementById('chapterSelect');
const toggleReferencesBtn = document.getElementById('toggleReferences');
const translationSelect = document.getElementById('translationSelect');
const toggleSidebarBtn = document.getElementById('toggleSidebar');
const sidebar = document.getElementById('sidebar');
const sidebarIcon = document.getElementById('sidebarIcon');
const contentWrapper = document.getElementById('contentWrapper');

// Update browser URL with current state
function updateURL(verseNum = null) {
    if (!currentBook || !currentChapter || !selectedTranslation) return;
    
    const params = new URLSearchParams();
    params.set('translation', selectedTranslation);
    params.set('book', currentBook);
    params.set('chapter', currentChapter);
    if (verseNum) {
        params.set('verse', verseNum);
    }
    
    const newUrl = `${window.location.pathname}?${params.toString()}`;
    window.history.pushState({}, '', newUrl);
}

// Scroll to a specific verse
function scrollToVerse(verseNum) {
    const verseElement = document.querySelector(`[data-verse="${verseNum}"]`);
    if (verseElement) {
        verseElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
        verseElement.style.backgroundColor = '#fef3c7';
        setTimeout(() => {
            verseElement.style.transition = 'background-color 2s';
            verseElement.style.backgroundColor = '';
        }, 1000);
    }
}

// Initialize the application
async function init() {
    await loadTranslations();
    
    // Check URL parameters first
    const urlParams = new URLSearchParams(window.location.search);
    const translationParam = urlParams.get('translation');
    const bookParam = urlParams.get('book');
    const chapterParam = urlParams.get('chapter');
    const verseParam = urlParams.get('verse');
    
    // Set translation from URL if provided
    if (translationParam && translationInfo[translationParam]) {
        selectedTranslation = translationParam;
        translationSelect.value = translationParam;
        updateHeader();
    }
    
    await loadBooks();
    setupEventListeners();
    
    if (bookParam && chapterParam) {
        const book = bibleBooks.find(b => b.name.toLowerCase() === bookParam.toLowerCase());
        if (book) {
            currentBook = book.name;
            currentChapter = parseInt(chapterParam);
            maxChapter = book.chapterCount;
            
            // Update chapter selector
            chapterSelect.innerHTML = '';
            for (let i = 1; i <= maxChapter; i++) {
                const option = document.createElement('option');
                option.value = i;
                option.textContent = `Chapter ${i}`;
                chapterSelect.appendChild(option);
            }
            chapterSelect.value = currentChapter;
            chapterSelector.classList.remove('hidden');
            
            await loadChapter();
            
            // Scroll to verse if provided
            if (verseParam) {
                setTimeout(() => scrollToVerse(parseInt(verseParam)), 300);
            }
        }
    }
}

// Load available translations
async function loadTranslations() {
    try {
        const response = await fetch('/api/translations');
        const translations = await response.json();
        
        translationSelect.innerHTML = '';
        translations.forEach(trans => {
            translationInfo[trans.name] = trans;
            const option = document.createElement('option');
            option.value = trans.name;
            option.textContent = trans.fullName;
            translationSelect.appendChild(option);
        });
        
        if (translations.length > 0) {
            selectedTranslation = translations[0].name;
            updateHeader();
        }
    } catch (error) {
        console.error('Error loading translations:', error);
        translationSelect.innerHTML = '<option value="">Error</option>';
    }
}

// Update the header with current translation info
function updateHeader() {
    const info = translationInfo[selectedTranslation];
    if (info) {
        document.getElementById('translationFullName').textContent = `Holy Bible - ${info.fullName}`;
        document.getElementById('translationDescription').textContent = info.description;
    }
}

// Load all available books
async function loadBooks() {
    try {
        const url = `/api/books?translation=${selectedTranslation}`;
        const response = await fetch(url);
        bibleBooks = await response.json();
        renderBookList();
    } catch (error) {
        console.error('Error loading books:', error);
        bookList.innerHTML = '<div class="text-red-500 text-sm">Error loading books</div>';
    }
}

// Render the book list in the sidebar
function renderBookList() {
    bookList.innerHTML = '';
    
    bibleBooks.forEach((book, index) => {
        const bookButton = document.createElement('button');
        bookButton.className = 'w-full text-left px-3 py-2 rounded-md hover:bg-blue-50 transition text-sm';
        bookButton.innerHTML = `
            <div class="font-medium text-gray-800">${book.name}</div>
            <div class="text-xs text-gray-500">${book.chapterCount} chapter${book.chapterCount > 1 ? 's' : ''}</div>
        `;
        bookButton.onclick = () => selectBook(book);
        
        // Highlight Old Testament vs New Testament sections
        if (index === 0) {
            const otLabel = document.createElement('div');
            otLabel.className = 'text-xs font-bold text-gray-600 mb-2 mt-0';
            otLabel.textContent = 'Old Testament';
            bookList.appendChild(otLabel);
        }
        
        // This is a simple heuristic - adjust based on actual OT/NT split
        if (book.name.toLowerCase().includes('matthew') && index > 0) {
            const ntLabel = document.createElement('div');
            ntLabel.className = 'text-xs font-bold text-gray-600 mb-2 mt-4';
            ntLabel.textContent = 'New Testament';
            bookList.appendChild(ntLabel);
        }
        
        bookList.appendChild(bookButton);
    });
}

// Select a book and load its first chapter
function selectBook(book) {
    currentBook = book.name;
    currentChapter = 1;
    maxChapter = book.chapterCount;
    
    // Update chapter selector
    chapterSelect.innerHTML = '';
    for (let i = 1; i <= maxChapter; i++) {
        const option = document.createElement('option');
        option.value = i;
        option.textContent = `Chapter ${i}`;
        chapterSelect.appendChild(option);
    }
    chapterSelect.value = 1;
    chapterSelector.classList.remove('hidden');
    
    loadChapter();
    updateURL();
}

// Load and display a specific chapter
async function loadChapter() {
    if (!currentBook) return;
    
    try {
        versesContainer.innerHTML = '<div class="text-center text-gray-500 py-8">Loading...</div>';
        
        const url = `/api/chapter?book=${encodeURIComponent(currentBook)}&chapter=${currentChapter}&translation=${selectedTranslation}`;
        const response = await fetch(url);
        const data = await response.json();
        
        // Update title
        chapterTitle.textContent = `${data.book} ${data.chapter}`;
        chapterSelect.value = currentChapter;
        
        // Render verses
        renderVerses(data.verses);
        
        // Update navigation buttons
        prevChapterBtn.disabled = currentChapter <= 1;
        nextChapterBtn.disabled = currentChapter >= maxChapter;
        
        // Update URL
        updateURL();
        
        // Scroll to top
        window.scrollTo({ top: 0, behavior: 'smooth' });
        
    } catch (error) {
        console.error('Error loading chapter:', error);
        versesContainer.innerHTML = '<div class="text-red-500 text-center py-8">Error loading chapter</div>';
    }
}

// Render verses in the container
function renderVerses(verses) {
    versesContainer.innerHTML = '';
    
    if (!verses || verses.length === 0) {
        versesContainer.innerHTML = '<div class="text-gray-500 text-center py-8">No verses found</div>';
        return;
    }
    
    // Add running header
    if (currentBook) {
        const runningHeader = document.createElement('div');
        runningHeader.className = 'running-header';
        runningHeader.textContent = `${currentBook} ${currentChapter}`;
        versesContainer.appendChild(runningHeader);
    }
    
    // Add chapter heading
    const chapterHeading = document.createElement('div');
    chapterHeading.className = 'chapter-heading text-2xl font-semibold mb-6 mt-4';
    chapterHeading.textContent = `Chapter ${currentChapter}`;
    versesContainer.appendChild(chapterHeading);
    
    const container = document.createElement('div');
    container.className = 'verse-container';
    if (!showReferences) {
        container.classList.add('hide-references');
    }
    
    verses.forEach((verseData) => {
        const verseNum = verseData.verse;
        const verseText = verseData.text || '';
        const sectionTitle = verseData.sectionTitle || '';
        const crossReferences = verseData.crossReferences || [];
        const notes = verseData.notes || [];
        
        // Display section title if present
        if (sectionTitle) {
            const titleDiv = document.createElement('div');
            titleDiv.className = 'section-title text-xl font-bold text-gray-900 mt-8 mb-4 border-b-2 border-gray-300 pb-2';
            titleDiv.textContent = sectionTitle;
            container.appendChild(titleDiv);
        }
        
        const verseDiv = document.createElement('div');
        verseDiv.className = 'verse';
        verseDiv.setAttribute('data-verse', verseNum);
        
        // Create verse URL
        const verseUrl = `${window.location.origin}/?translation=${selectedTranslation}&book=${encodeURIComponent(currentBook)}&chapter=${currentChapter}&verse=${verseNum}`;
        
        let html = `
            <a href="${verseUrl}" class="verse-number" title="Click to copy link, right-click to open in new tab">${verseNum}</a>
            <span class="verse-text">${formatVerseText(verseText, verseNum)}</span>
        `;
        
        // Add cross-references if they exist and should be shown
        if (crossReferences.length > 0 && showReferences) {
            html += '<div class="verse-references">';
            html += '<span class="font-semibold">Cross-references: </span>';
            crossReferences.forEach((ref, refIdx) => {
                const marker = ref.marker ? `[${ref.marker}] ` : '';
                const refText = ref.text || ref; // Handle both old string format and new object format
                html += `<span class="reference">${marker}${createReferenceLink(refText)}</span>`;
                if (refIdx < crossReferences.length - 1) {
                    html += '<span class="mx-1">;</span>';
                }
            });
            html += '</div>';
        }
        
        // Add explanatory notes if they exist and should be shown
        if (notes.length > 0 && showReferences) {
            notes.forEach(note => {
                const marker = note.marker ? `[${note.marker}] ` : '';
                const noteText = note.text || note; // Handle both old string format and new object format
                html += `<div class="verse-references">
                    <span class="font-semibold">${marker}</span>${escapeHtml(noteText)}
                </div>`;
            });
        }
        
        verseDiv.innerHTML = html;
        container.appendChild(verseDiv);
    });
    
    versesContainer.appendChild(container);
}

// Parse a reference string and create a clickable link
function createReferenceLink(refText) {
    // Match patterns like "John 3:16", "1 Cor 8:6", "Gen 1:1", "Ps 102:25", etc.
    const refPattern = /([1-3]?\s*[A-Za-z]+\.?)\s+(\d+):(\d+)/g;
    let html = '';
    let lastIndex = 0;
    let match;
    
    while ((match = refPattern.exec(refText)) !== null) {
        // Add any text before the match (like semicolons, commas)
        if (match.index > lastIndex) {
            html += escapeHtml(refText.substring(lastIndex, match.index));
        }
        
        const bookName = match[1].trim();
        const chapter = match[2];
        const verse = match[3];
        
        // Create a clickable link
        html += `<a href="#" class="reference-link" data-book="${escapeHtml(bookName)}" data-chapter="${chapter}" data-verse="${verse}">${escapeHtml(match[0])}</a>`;
        
        lastIndex = match.index + match[0].length;
    }
    
    // Add any remaining text
    if (lastIndex < refText.length) {
        html += escapeHtml(refText.substring(lastIndex));
    }
    
    return html || escapeHtml(refText);
}

// Format verse text - allows <i> and <sup> tags, escapes everything else
function formatVerseText(text, verseNum) {
    if (!text) return '';
    
    // Split on <i>, </i>, <sup>, and </sup> tags
    const parts = text.split(/(<\/?i>|<\/?sup>)/);
    let html = '';
    
    for (let i = 0; i < parts.length; i++) {
        const part = parts[i];
        
        if (part === '<i>' || part === '</i>' || part === '<sup>' || part === '</sup>') {
            html += part;
        } else if (part) {
            html += escapeHtml(part);
        }
    }
    
    return html;
}

// Navigate to a specific book and chapter
function navigateToReference(bookName, chapter) {
    // Try to find matching book (handle abbreviations)
    const bookMap = {
        'Gen': 'Genesis', 'Gen.': 'Genesis',
        'Ex': 'Exodus', 'Exod': 'Exodus', 'Exod.': 'Exodus',
        'Lev': 'Leviticus', 'Lev.': 'Leviticus',
        'Num': 'Numbers', 'Num.': 'Numbers',
        'Deut': 'Deuteronomy', 'Deut.': 'Deuteronomy',
        'Josh': 'Joshua', 'Josh.': 'Joshua',
        'Judg': 'Judges', 'Judg.': 'Judges',
        'Ruth': 'Ruth',
        '1 Sam': '1 Samuel', '1Sam': '1 Samuel', '1 Sam.': '1 Samuel',
        '2 Sam': '2 Samuel', '2Sam': '2 Samuel', '2 Sam.': '2 Samuel',
        '1 Kin': '1 Kings', '1Kin': '1 Kings', '1 Kgs': '1 Kings', '1 Kings': '1 Kings',
        '2 Kin': '2 Kings', '2Kin': '2 Kings', '2 Kgs': '2 Kings', '2 Kings': '2 Kings',
        '1 Chr': '1 Chronicles', '1Chr': '1 Chronicles', '1 Chron': '1 Chronicles',
        '2 Chr': '2 Chronicles', '2Chr': '2 Chronicles', '2 Chron': '2 Chronicles',
        'Ezra': 'Ezra', 'Neh': 'Nehemiah', 'Neh.': 'Nehemiah',
        'Esth': 'Esther', 'Esth.': 'Esther',
        'Job': 'Job',
        'Ps': 'Psalms', 'Psa': 'Psalms', 'Psalm': 'Psalms',
        'Prov': 'Proverbs', 'Prov.': 'Proverbs',
        'Eccl': 'Ecclesiastes', 'Eccl.': 'Ecclesiastes',
        'Song': 'Song of Solomon', 'Song.': 'Song of Solomon',
        'Is': 'Isaiah', 'Isa': 'Isaiah', 'Isa.': 'Isaiah', 'Is.': 'Isaiah',
        'Jer': 'Jeremiah', 'Jer.': 'Jeremiah',
        'Lam': 'Lamentations', 'Lam.': 'Lamentations',
        'Ezek': 'Ezekiel', 'Ezek.': 'Ezekiel',
        'Dan': 'Daniel', 'Dan.': 'Daniel',
        'Hos': 'Hosea', 'Hos.': 'Hosea',
        'Joel': 'Joel',
        'Amos': 'Amos',
        'Obad': 'Obadiah', 'Obad.': 'Obadiah',
        'Jon': 'Jonah', 'Jonah': 'Jonah',
        'Mic': 'Micah', 'Mic.': 'Micah',
        'Nah': 'Nahum', 'Nah.': 'Nahum',
        'Hab': 'Habakkuk', 'Hab.': 'Habakkuk',
        'Zeph': 'Zephaniah', 'Zeph.': 'Zephaniah',
        'Hag': 'Haggai', 'Hag.': 'Haggai',
        'Zech': 'Zechariah', 'Zech.': 'Zechariah',
        'Mal': 'Malachi', 'Mal.': 'Malachi',
        'Matt': 'Matthew', 'Matt.': 'Matthew',
        'Mark': 'Mark',
        'Luke': 'Luke',
        'John': 'John',
        'Acts': 'Acts',
        'Rom': 'Romans', 'Rom.': 'Romans',
        '1 Cor': '1 Corinthians', '1Cor': '1 Corinthians', '1 Cor.': '1 Corinthians',
        '2 Cor': '2 Corinthians', '2Cor': '2 Corinthians', '2 Cor.': '2 Corinthians',
        'Gal': 'Galatians', 'Gal.': 'Galatians',
        'Eph': 'Ephesians', 'Eph.': 'Ephesians',
        'Phil': 'Philippians', 'Phil.': 'Philippians',
        'Col': 'Colossians', 'Col.': 'Colossians',
        '1 Thess': '1 Thessalonians', '1Thess': '1 Thessalonians', '1 Thess.': '1 Thessalonians',
        '2 Thess': '2 Thessalonians', '2Thess': '2 Thessalonians', '2 Thess.': '2 Thessalonians',
        '1 Tim': '1 Timothy', '1Tim': '1 Timothy', '1 Tim.': '1 Timothy',
        '2 Tim': '2 Timothy', '2Tim': '2 Timothy', '2 Tim.': '2 Timothy',
        'Titus': 'Titus',
        'Philem': 'Philemon', 'Phlm': 'Philemon', 'Phlm.': 'Philemon',
        'Heb': 'Hebrews', 'Heb.': 'Hebrews',
        'James': 'James', 'Jas': 'James', 'Jas.': 'James',
        '1 Pet': '1 Peter', '1Pet': '1 Peter', '1 Pet.': '1 Peter',
        '2 Pet': '2 Peter', '2Pet': '2 Peter', '2 Pet.': '2 Peter',
        '1 John': '1 John', '1John': '1 John',
        '2 John': '2 John', '2John': '2 John',
        '3 John': '3 John', '3John': '3 John',
        'Jude': 'Jude',
        'Rev': 'Revelation', 'Rev.': 'Revelation'
    };
    
    let fullBookName = bookMap[bookName] || bookName;
    
    // Try to find the book
    const book = bibleBooks.find(b => 
        b.name.toLowerCase() === fullBookName.toLowerCase() ||
        b.name.toLowerCase().startsWith(fullBookName.toLowerCase())
    );
    
    if (book) {
        // Open in new tab with translation parameter
        window.open(`/?translation=${selectedTranslation}&book=${encodeURIComponent(book.name)}&chapter=${chapter}`, '_blank');
    }
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Setup event listeners
function setupEventListeners() {
    prevChapterBtn.addEventListener('click', () => {
        if (currentChapter > 1) {
            currentChapter--;
            loadChapter();
        }
    });
    
    nextChapterBtn.addEventListener('click', () => {
        if (currentChapter < maxChapter) {
            currentChapter++;
            loadChapter();
        }
    });
    
    chapterSelect.addEventListener('change', (e) => {
        const newChapter = parseInt(e.target.value);
        if (newChapter && newChapter !== currentChapter) {
            currentChapter = newChapter;
            loadChapter();
        }
    });
    
    // Translation selector change
    translationSelect.addEventListener('change', async (e) => {
        selectedTranslation = e.target.value;
        updateHeader();
        updateURL();
        await loadBooks();
        // Reset to first book if current book doesn't exist in new translation
        if (currentBook) {
            const bookExists = bibleBooks.find(b => b.name === currentBook);
            if (bookExists) {
                loadChapter();
            } else {
                currentBook = null;
                currentChapter = 1;
                chapterTitle.textContent = 'Select a book to begin';
                versesContainer.innerHTML = '';
                chapterSelector.classList.add('hidden');
                updateURL();
            }
        }
    });
    
    // Toggle references button
    if (toggleReferencesBtn) {
        toggleReferencesBtn.addEventListener('click', () => {
            showReferences = !showReferences;
            toggleReferencesBtn.innerHTML = showReferences 
                ? '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"></path></svg> Hide References'
                : '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21"></path></svg> Show References';
            loadChapter(); // Reload to apply change
        });
    }
    
    // Toggle sidebar button
    if (toggleSidebarBtn) {
        toggleSidebarBtn.addEventListener('click', () => {
            sidebarVisible = !sidebarVisible;
            if (sidebarVisible) {
                sidebar.classList.remove('collapsed');
                contentWrapper.classList.remove('sidebar-collapsed');
                toggleSidebarBtn.classList.remove('sidebar-hidden');
                sidebarIcon.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>';
            } else {
                sidebar.classList.add('collapsed');
                contentWrapper.classList.add('sidebar-collapsed');
                toggleSidebarBtn.classList.add('sidebar-hidden');
                sidebarIcon.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>';
            }
        });
    }
    
    // Handle reference link clicks with event delegation
    versesContainer.addEventListener('click', (e) => {
        if (e.target.classList.contains('reference-link')) {
            e.preventDefault();
            const bookName = e.target.dataset.book;
            const chapter = e.target.dataset.chapter;
            navigateToReference(bookName, chapter);
        }
        
        // Handle verse number clicks - copy to clipboard on left-click
        if (e.target.classList.contains('verse-number')) {
            // Only prevent default on left-click (not right-click or middle-click)
            if (e.button === 0) {
                e.preventDefault();
                const verseUrl = e.target.href;
                
                // Copy to clipboard
                navigator.clipboard.writeText(verseUrl).then(() => {
                    // Show temporary feedback
                    const originalText = e.target.textContent;
                    const originalColor = e.target.style.color;
                    e.target.textContent = '✓';
                    e.target.style.color = '#059669';
                    
                    setTimeout(() => {
                        e.target.textContent = originalText;
                        e.target.style.color = originalColor;
                    }, 1000);
                }).catch(err => {
                    console.error('Failed to copy:', err);
                    alert('Failed to copy link to clipboard');
                });
            }
        }
    });
    
    // Keyboard navigation
    document.addEventListener('keydown', (e) => {
        if (e.key === 'ArrowLeft' && !prevChapterBtn.disabled) {
            currentChapter--;
            loadChapter();
        } else if (e.key === 'ArrowRight' && !nextChapterBtn.disabled) {
            currentChapter++;
            loadChapter();
        }
    });
}

// Start the application
init();
