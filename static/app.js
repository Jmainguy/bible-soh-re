// State management
let bibleBooks = [];
let currentBook = null;
let currentChapter = 1;
let maxChapter = 1;
let showReferences = false; // Toggle for showing/hiding references
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
const sidebarOverlay = document.getElementById('sidebarOverlay');

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

// Scroll to a specific verse or verse range
function scrollToVerse(verseNum) {
    // Handle verse ranges (e.g., "20-23")
    let verses = [];
    if (typeof verseNum === 'string' && verseNum.includes('-')) {
        const [start, end] = verseNum.split('-').map(v => parseInt(v.trim()));
        for (let i = start; i <= end; i++) {
            verses.push(i);
        }
    } else {
        verses = [parseInt(verseNum)];
    }
    
    // Highlight all verses in the range
    verses.forEach((v, index) => {
        const verseElement = document.querySelector(`[data-verse="${v}"]`);
        if (verseElement) {
            // Scroll to the first verse in range
            if (index === 0) {
                verseElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
            }
            verseElement.style.backgroundColor = '#fef3c7';
            setTimeout(() => {
                verseElement.style.transition = 'background-color 2s';
                verseElement.style.backgroundColor = '';
            }, 1000);
        }
    });
}

// Initialize the application
async function init() {
    // Check authentication status
    await checkAuthStatus();
    
    // Hide sidebar on mobile by default
    if (window.innerWidth <= 1024) {
        sidebar.classList.add('collapsed');
        contentWrapper.classList.add('sidebar-collapsed');
        toggleSidebarBtn.classList.add('sidebar-hidden');
        sidebarIcon.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>';
        sidebarVisible = false;
    }
    
    await loadTranslations();
    
    // Check URL parameters first
    const urlParams = new URLSearchParams(window.location.search);
    const translationParam = urlParams.get('translation');
    const bookParam = urlParams.get('book');
    const chapterParam = urlParams.get('chapter');
    const verseParam = urlParams.get('verse');
    
    // Set translation from URL if provided, otherwise default to NASB
    if (translationParam && translationInfo[translationParam]) {
        selectedTranslation = translationParam;
        translationSelect.value = translationParam;
        updateHeader();
    } else if (translationInfo['nasb']) {
        selectedTranslation = 'nasb';
        translationSelect.value = 'nasb';
        updateHeader();
    }
    
    await loadBooks();
    setupEventListeners();
    
    // Priority: URL params > saved position > default (Genesis 1)
    if (bookParam && chapterParam) {
        // URL parameters have highest priority
        const book = bibleBooks.find(b => b.name.toLowerCase() === bookParam.toLowerCase());
        if (book) {
            currentBook = book.name;
            currentChapter = parseInt(chapterParam);
            maxChapter = book.chapterCount;
            
            updateChapterSelector();
            
            await loadChapter();
            
            // Scroll to verse or verse range if provided
            if (verseParam) {
                setTimeout(() => scrollToVerse(verseParam), 300);
            }
        }
    } else {
        // Try to load saved position
        const savedPosition = await loadSavedPosition();
        
        if (savedPosition) {
            // Use saved position
            const book = bibleBooks.find(b => b.name === savedPosition.book);
            if (book && translationInfo[savedPosition.translation]) {
                currentBook = savedPosition.book;
                currentChapter = savedPosition.chapter;
                maxChapter = book.chapterCount;
                selectedTranslation = savedPosition.translation;
                translationSelect.value = savedPosition.translation;
                updateHeader();
                
                updateChapterSelector();
                
                await loadChapter();
                console.log(`Restored reading position from ${savedPosition.source}: ${currentBook} ${currentChapter} (${selectedTranslation})`);
            } else {
                // Saved position is invalid, load default
                loadDefaultPosition();
            }
        } else {
            // No saved position, load default
            loadDefaultPosition();
        }
    }
}

// Load default position (Genesis 1)
async function loadDefaultPosition() {
    const genesis = bibleBooks.find(b => b.name === 'Genesis');
    if (genesis) {
        currentBook = 'Genesis';
        currentChapter = 1;
        maxChapter = genesis.chapterCount;
        
        updateChapterSelector();
        
        await loadChapter();
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
        bookButton.setAttribute('data-book-name', book.name);
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

// Update the highlighted book in the sidebar
function updateBookHighlight() {
    // Remove highlight from all books
    const allBookButtons = bookList.querySelectorAll('button[data-book-name]');
    allBookButtons.forEach(btn => {
        btn.classList.remove('bg-blue-100', 'border-2', 'border-blue-500');
        btn.classList.add('hover:bg-blue-50');
    });
    
    // Highlight current book
    if (currentBook) {
        const currentButton = bookList.querySelector(`button[data-book-name="${currentBook}"]`);
        if (currentButton) {
            currentButton.classList.add('bg-blue-100', 'border-2', 'border-blue-500');
            currentButton.classList.remove('hover:bg-blue-50');
            
            // Scroll the book into view in the sidebar
            currentButton.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
    }
}

// Select a book and load its first chapter
function selectBook(book) {
    currentBook = book.name;
    currentChapter = 1;
    maxChapter = book.chapterCount;
    
    updateChapterSelector();
    updateBookHighlight();
    
    loadChapter();
    updateURL();
    
    // Auto-hide sidebar on mobile after selection
    if (window.innerWidth <= 1024 && sidebarVisible) {
        sidebar.classList.add('collapsed');
        contentWrapper.classList.add('sidebar-collapsed');
        toggleSidebarBtn.classList.add('sidebar-hidden');
        sidebarOverlay.classList.remove('active');
        sidebarIcon.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>';
        sidebarVisible = false;
    }
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
        
        // Update navigation buttons - only disable at absolute boundaries
        const currentIndex = bibleBooks.findIndex(b => b.name === currentBook);
        prevChapterBtn.disabled = (currentChapter <= 1 && currentIndex <= 0);
        nextChapterBtn.disabled = (currentChapter >= maxChapter && currentIndex >= bibleBooks.length - 1);
        
        // Update book highlighting in sidebar
        updateBookHighlight();
        
        // Update URL
        updateURL();
        
        // Save reading position
        saveReadingPosition();
        
        // Load notes for this chapter (if user is authenticated)
        loadNotes();
        
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
    
    // Check if first verse has an intro note (chapter summary/description)
    if (verses.length > 0 && verses[0].introNote) {
        const summaryDiv = document.createElement('div');
        summaryDiv.className = 'chapter-summary text-gray-600 italic mb-6 pb-4 border-b border-gray-300';
        summaryDiv.textContent = verses[0].introNote;
        versesContainer.appendChild(summaryDiv);
    }
    
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
        const studyNotes = verseData.studyNotes || [];
        
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
                    <span class="font-semibold">${marker}</span>${createReferenceLink(noteText)}
                </div>`;
            });
        }
        
        // Add study notes (KJV marginal notes) if they exist and should be shown
        if (studyNotes.length > 0 && showReferences) {
            studyNotes.forEach(note => {
                const marker = note.marker ? `[${note.marker}] ` : '';
                const noteText = note.text || note; // Handle both old string format and new object format
                html += `<div class="verse-references">
                    <span class="font-semibold">Study note: ${marker}</span>${createReferenceLink(noteText)}
                </div>`;
            });
        }
        
        verseDiv.innerHTML = html;
        container.appendChild(verseDiv);
    });
    
    versesContainer.appendChild(container);
}

// Map book abbreviations to full names
function getFullBookName(bookName) {
    const bookMap = {
        'gen': 'Genesis', 'gen.': 'Genesis',
        'ex': 'Exodus', 'exod': 'Exodus', 'exod.': 'Exodus', 'exo': 'Exodus',
        'lev': 'Leviticus', 'lev.': 'Leviticus',
        'num': 'Numbers', 'num.': 'Numbers', 'numb': 'Numbers',
        'deut': 'Deuteronomy', 'deut.': 'Deuteronomy', 'deu': 'Deuteronomy', 'dt': 'Deuteronomy',
        'josh': 'Joshua', 'josh.': 'Joshua', 'jos': 'Joshua',
        'judg': 'Judges', 'judg.': 'Judges', 'jdg': 'Judges',
        'ruth': 'Ruth', 'rut': 'Ruth',
        '1 sam': '1 Samuel', '1sam': '1 Samuel', '1 sam.': '1 Samuel', '1sa': '1 Samuel', '1 sa': '1 Samuel',
        '2 sam': '2 Samuel', '2sam': '2 Samuel', '2 sam.': '2 Samuel', '2sa': '2 Samuel', '2 sa': '2 Samuel',
        '1 kin': '1 Kings', '1kin': '1 Kings', '1 kgs': '1 Kings', '1 kings': '1 Kings', '1ki': '1 Kings', '1 ki': '1 Kings',
        '2 kin': '2 Kings', '2kin': '2 Kings', '2 kgs': '2 Kings', '2 kings': '2 Kings', '2ki': '2 Kings', '2 ki': '2 Kings',
        '1 chr': '1 Chronicles', '1chr': '1 Chronicles', '1 chron': '1 Chronicles', '1ch': '1 Chronicles', '1 ch': '1 Chronicles',
        '2 chr': '2 Chronicles', '2chr': '2 Chronicles', '2 chron': '2 Chronicles', '2ch': '2 Chronicles', '2 ch': '2 Chronicles',
        'ezra': 'Ezra', 'ezr': 'Ezra',
        'neh': 'Nehemiah', 'neh.': 'Nehemiah', 'ne': 'Nehemiah',
        'esth': 'Esther', 'esth.': 'Esther', 'est': 'Esther', 'es': 'Esther',
        'job': 'Job',
        'ps': 'Psalms', 'psa': 'Psalms', 'psalm': 'Psalms', 'pss': 'Psalms',
        'prov': 'Proverbs', 'prov.': 'Proverbs', 'pro': 'Proverbs', 'prv': 'Proverbs',
        'eccl': 'Ecclesiastes', 'eccl.': 'Ecclesiastes', 'ecc': 'Ecclesiastes', 'qoh': 'Ecclesiastes',
        'song': 'Song of Solomon', 'song.': 'Song of Solomon', 'sos': 'Song of Solomon', 'so': 'Song of Solomon',
        'is': 'Isaiah', 'isa': 'Isaiah', 'isa.': 'Isaiah', 'is.': 'Isaiah',
        'jer': 'Jeremiah', 'jer.': 'Jeremiah', 'je': 'Jeremiah',
        'lam': 'Lamentations', 'lam.': 'Lamentations', 'la': 'Lamentations',
        'ezek': 'Ezekiel', 'ezek.': 'Ezekiel', 'eze': 'Ezekiel', 'ezk': 'Ezekiel',
        'dan': 'Daniel', 'dan.': 'Daniel', 'da': 'Daniel', 'dn': 'Daniel',
        'hos': 'Hosea', 'hos.': 'Hosea', 'ho': 'Hosea',
        'joel': 'Joel', 'joe': 'Joel', 'jl': 'Joel',
        'amos': 'Amos', 'am': 'Amos',
        'obad': 'Obadiah', 'obad.': 'Obadiah', 'oba': 'Obadiah', 'ob': 'Obadiah',
        'jon': 'Jonah', 'jonah': 'Jonah', 'jnh': 'Jonah',
        'mic': 'Micah', 'mic.': 'Micah', 'mi': 'Micah',
        'nah': 'Nahum', 'nah.': 'Nahum', 'na': 'Nahum',
        'hab': 'Habakkuk', 'hab.': 'Habakkuk', 'hb': 'Habakkuk',
        'zeph': 'Zephaniah', 'zeph.': 'Zephaniah', 'zep': 'Zephaniah',
        'hag': 'Haggai', 'hag.': 'Haggai', 'hg': 'Haggai',
        'zech': 'Zechariah', 'zech.': 'Zechariah', 'zec': 'Zechariah', 'zch': 'Zechariah',
        'mal': 'Malachi', 'mal.': 'Malachi',
        'matt': 'Matthew', 'matt.': 'Matthew', 'mt': 'Matthew', 'mat': 'Matthew',
        'mark': 'Mark', 'mk': 'Mark', 'mar': 'Mark', 'mrk': 'Mark',
        'luke': 'Luke', 'lk': 'Luke', 'luk': 'Luke',
        'john': 'John', 'jn': 'John', 'jhn': 'John',
        'acts': 'Acts', 'act': 'Acts', 'ac': 'Acts',
        'rom': 'Romans', 'rom.': 'Romans', 'ro': 'Romans',
        '1 cor': '1 Corinthians', '1cor': '1 Corinthians', '1 cor.': '1 Corinthians', '1co': '1 Corinthians', '1 co': '1 Corinthians',
        '2 cor': '2 Corinthians', '2cor': '2 Corinthians', '2 cor.': '2 Corinthians', '2co': '2 Corinthians', '2 co': '2 Corinthians',
        'gal': 'Galatians', 'gal.': 'Galatians', 'ga': 'Galatians',
        'eph': 'Ephesians', 'eph.': 'Ephesians', 'ephes': 'Ephesians',
        'phil': 'Philippians', 'phil.': 'Philippians', 'php': 'Philippians', 'ph': 'Philippians',
        'col': 'Colossians', 'col.': 'Colossians',
        '1 thess': '1 Thessalonians', '1thess': '1 Thessalonians', '1 thess.': '1 Thessalonians', '1th': '1 Thessalonians', '1 th': '1 Thessalonians',
        '2 thess': '2 Thessalonians', '2thess': '2 Thessalonians', '2 thess.': '2 Thessalonians', '2th': '2 Thessalonians', '2 th': '2 Thessalonians',
        '1 tim': '1 Timothy', '1tim': '1 Timothy', '1 tim.': '1 Timothy', '1ti': '1 Timothy', '1 ti': '1 Timothy',
        '2 tim': '2 Timothy', '2tim': '2 Timothy', '2 tim.': '2 Timothy', '2ti': '2 Timothy', '2 ti': '2 Timothy',
        'titus': 'Titus', 'tit': 'Titus', 'ti': 'Titus',
        'philem': 'Philemon', 'phlm': 'Philemon', 'phlm.': 'Philemon', 'phm': 'Philemon',
        'heb': 'Hebrews', 'heb.': 'Hebrews',
        'james': 'James', 'jas': 'James', 'jas.': 'James', 'jam': 'James', 'jm': 'James',
        '1 pet': '1 Peter', '1pet': '1 Peter', '1 pet.': '1 Peter', '1pe': '1 Peter', '1 pe': '1 Peter', '1p': '1 Peter', '1 p': '1 Peter',
        '2 pet': '2 Peter', '2pet': '2 Peter', '2 pet.': '2 Peter', '2pe': '2 Peter', '2 pe': '2 Peter', '2p': '2 Peter', '2 p': '2 Peter',
        '1 john': '1 John', '1john': '1 John', '1jn': '1 John', '1 jn': '1 John', '1j': '1 John', '1 j': '1 John',
        '2 john': '2 John', '2john': '2 John', '2jn': '2 John', '2 jn': '2 John', '2j': '2 John', '2 j': '2 John',
        '3 john': '3 John', '3john': '3 John', '3jn': '3 John', '3 jn': '3 John', '3j': '3 John', '3 j': '3 John',
        'jude': 'Jude', 'jud': 'Jude',
        'rev': 'Revelation', 'rev.': 'Revelation', 're': 'Revelation'
    };
    
    return bookMap[bookName.toLowerCase()] || bookName;
}

// Parse a reference string and create a clickable link
function createReferenceLink(refText) {
    // Parse complex references like "Ps 22:27; 86:9; Is 49:22; 23; 60:3; Zech 8:20-23"
    // Split by semicolon to get individual references
    const refParts = refText.split(';');
    let html = '';
    let currentBook = null;
    let currentChapter = null;
    
    for (let i = 0; i < refParts.length; i++) {
        const part = refParts[i].trim();
        if (!part) continue;
        
        // Add semicolon separator between references (except first)
        if (i > 0) {
            html += '; ';
        }
        
        // Full reference with book: "Ps 22:27", "Is 9:6f" (f means "and following")
        const fullRefMatch = part.match(/^([1-3]?\s*[A-Za-z]+\.?)\s+(\d+):(\d+(?:-\d+)?)(f{1,2})?$/);
        if (fullRefMatch) {
            currentBook = fullRefMatch[1].trim();
            currentChapter = fullRefMatch[2];
            let verse = fullRefMatch[3];
            const followingSuffix = fullRefMatch[4]; // "f" or "ff"
            
            // Convert "f" to range (f = next verse, ff = next 2+ verses)
            if (followingSuffix) {
                const baseVerse = parseInt(verse);
                const endVerse = followingSuffix === 'ff' ? baseVerse + 2 : baseVerse + 1;
                verse = `${baseVerse}-${endVerse}`;
            }
            
            html += createSingleReferenceLink(currentBook, currentChapter, verse, part);
            continue;
        }
        
        // Chapter:verse reference (implies same book): "86:9" or "89:3f"
        const chapterVerseMatch = part.match(/^(\d+):(\d+(?:-\d+)?)(f{1,2})?$/);
        if (chapterVerseMatch && currentBook) {
            currentChapter = chapterVerseMatch[1];
            let verse = chapterVerseMatch[2];
            const followingSuffix = chapterVerseMatch[3];
            
            // Convert "f" to range
            if (followingSuffix) {
                const baseVerse = parseInt(verse);
                const endVerse = followingSuffix === 'ff' ? baseVerse + 2 : baseVerse + 1;
                verse = `${baseVerse}-${endVerse}`;
            }
            
            html += createSingleReferenceLink(currentBook, currentChapter, verse, part);
            continue;
        }
        
        // Verse-only reference (implies same book and chapter): "23" or "20-23"
        const verseOnlyMatch = part.match(/^(\d+(?:-\d+)?)$/);
        if (verseOnlyMatch && currentBook && currentChapter) {
            const verse = verseOnlyMatch[1];
            html += createSingleReferenceLink(currentBook, currentChapter, verse, part);
            continue;
        }
        
        // If no match, just add as plain text
        html += escapeHtml(part);
    }
    
    return html || escapeHtml(refText);
}

// Helper function to create a single reference link
function createSingleReferenceLink(bookAbbrev, chapter, verse, displayText) {
    const fullBookName = getFullBookName(bookAbbrev);
    
    // Check if the book exists in the current translation
    const bookExists = bibleBooks.some(b => b.name.toLowerCase() === fullBookName.toLowerCase());
    
    // If book doesn't exist in current translation, use NASB as fallback
    const translationToUse = bookExists ? selectedTranslation : 'nasb';
    
    const refUrl = `${window.location.origin}/?translation=${translationToUse}&book=${encodeURIComponent(fullBookName)}&chapter=${chapter}&verse=${verse}`;
    return `<a href="${refUrl}" target="_blank" class="reference-link" data-book="${escapeHtml(fullBookName)}" data-chapter="${chapter}" data-verse="${verse}">${escapeHtml(displayText)}</a>`;
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

// Navigate to a specific book and chapter (and optionally verse or verse range)
function navigateToReference(bookName, chapter, verse = null) {
    // Get full book name from abbreviation
    let fullBookName = getFullBookName(bookName);
    
    // Try to find the book
    const book = bibleBooks.find(b => 
        b.name.toLowerCase() === fullBookName.toLowerCase() ||
        b.name.toLowerCase().startsWith(fullBookName.toLowerCase())
    );
    
    if (book) {
        currentBook = book.name;
        currentChapter = parseInt(chapter);
        maxChapter = book.chapterCount;
        updateChapterSelector();
        loadChapter().then(() => {
            if (verse) {
                // Handle verse ranges (e.g., "20-23") or single verses
                const verseParam = verse.toString();
                // Update URL with verse parameter (keep range format if present)
                if (verseParam.includes('-')) {
                    const params = new URLSearchParams();
                    params.set('translation', selectedTranslation);
                    params.set('book', currentBook);
                    params.set('chapter', currentChapter);
                    params.set('verse', verseParam);
                    const newUrl = `${window.location.pathname}?${params.toString()}`;
                    window.history.pushState({}, '', newUrl);
                } else {
                    updateURL(parseInt(verse));
                }
                // Wait for DOM to update, then scroll to verse/range
                setTimeout(() => scrollToVerse(verseParam), 100);
            } else {
                updateURL();
            }
        });
    }
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Navigate to next book
function goToNextBook() {
    const currentIndex = bibleBooks.findIndex(b => b.name === currentBook);
    if (currentIndex >= 0 && currentIndex < bibleBooks.length - 1) {
        const nextBook = bibleBooks[currentIndex + 1];
        currentBook = nextBook.name;
        currentChapter = 1;
        maxChapter = nextBook.chapterCount;
        updateChapterSelector();
        updateBookHighlight();
        loadChapter();
        return true;
    }
    return false;
}

// Navigate to previous book
function goToPreviousBook() {
    const currentIndex = bibleBooks.findIndex(b => b.name === currentBook);
    if (currentIndex > 0) {
        const prevBook = bibleBooks[currentIndex - 1];
        currentBook = prevBook.name;
        currentChapter = prevBook.chapterCount; // Go to last chapter of previous book
        maxChapter = prevBook.chapterCount;
        updateChapterSelector();
        updateBookHighlight();
        loadChapter();
        return true;
    }
    return false;
}

// Update chapter selector dropdown
function updateChapterSelector() {
    chapterSelect.innerHTML = '';
    for (let i = 1; i <= maxChapter; i++) {
        const option = document.createElement('option');
        option.value = i;
        option.textContent = `Chapter ${i}`;
        chapterSelect.appendChild(option);
    }
    chapterSelect.value = currentChapter;
    chapterSelector.classList.remove('hidden');
}

// Setup event listeners
function setupEventListeners() {
    prevChapterBtn.addEventListener('click', () => {
        if (currentChapter > 1) {
            currentChapter--;
            loadChapter();
        } else {
            // Go to previous book if at chapter 1
            goToPreviousBook();
        }
    });
    
    nextChapterBtn.addEventListener('click', () => {
        if (currentChapter < maxChapter) {
            currentChapter++;
            loadChapter();
        } else {
            // Go to next book if at last chapter
            goToNextBook();
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
        // Set initial button text based on default state
        toggleReferencesBtn.innerHTML = showReferences 
            ? '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"></path></svg> Hide References'
            : '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21"></path></svg> Show References';
        
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
                sidebarOverlay.classList.add('active');
                sidebarIcon.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>';
            } else {
                sidebar.classList.add('collapsed');
                contentWrapper.classList.add('sidebar-collapsed');
                toggleSidebarBtn.classList.add('sidebar-hidden');
                sidebarOverlay.classList.remove('active');
                sidebarIcon.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>';
            }
        });
    }
    
    // Close sidebar when clicking overlay
    if (sidebarOverlay) {
        sidebarOverlay.addEventListener('click', () => {
            if (sidebarVisible && window.innerWidth <= 1024) {
                sidebar.classList.add('collapsed');
                contentWrapper.classList.add('sidebar-collapsed');
                toggleSidebarBtn.classList.add('sidebar-hidden');
                sidebarOverlay.classList.remove('active');
                sidebarIcon.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>';
                sidebarVisible = false;
            }
        });
    }
    
    // Handle reference link clicks with event delegation
    versesContainer.addEventListener('click', (e) => {
        if (e.target.classList.contains('reference-link')) {
            // Let the browser handle the link naturally (opens in new tab due to target="_blank")
            return;
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
            if (currentChapter > 1) {
                currentChapter--;
                loadChapter();
            } else {
                goToPreviousBook();
            }
        } else if (e.key === 'ArrowRight' && !nextChapterBtn.disabled) {
            if (currentChapter < maxChapter) {
                currentChapter++;
                loadChapter();
            } else {
                goToNextBook();
            }
        }
    });
    
    // Touch/swipe navigation for mobile
    let touchStartX = 0;
    let touchStartY = 0;
    let touchEndX = 0;
    let touchEndY = 0;
    
    const handleSwipe = () => {
        const diffX = touchEndX - touchStartX;
        const diffY = touchEndY - touchStartY;
        const minSwipeDistance = 50;
        const maxVerticalMovement = 80; // Maximum vertical movement allowed for horizontal swipe
        
        // Only trigger if horizontal swipe is greater than vertical (to avoid interfering with scroll)
        // AND vertical movement is minimal (to prevent triggering during scrolling)
        if (Math.abs(diffX) > Math.abs(diffY) && 
            Math.abs(diffX) > minSwipeDistance && 
            Math.abs(diffY) < maxVerticalMovement) {
            if (diffX > 0) {
                // Swipe right - go to previous chapter
                if (!prevChapterBtn.disabled) {
                    if (currentChapter > 1) {
                        currentChapter--;
                        loadChapter();
                    } else {
                        goToPreviousBook();
                    }
                }
            } else {
                // Swipe left - go to next chapter
                if (!nextChapterBtn.disabled) {
                    if (currentChapter < maxChapter) {
                        currentChapter++;
                        loadChapter();
                    } else {
                        goToNextBook();
                    }
                }
            }
        }
    };
    
    versesContainer.addEventListener('touchstart', (e) => {
        touchStartX = e.changedTouches[0].screenX;
        touchStartY = e.changedTouches[0].screenY;
    }, { passive: true });
    
    versesContainer.addEventListener('touchend', (e) => {
        touchEndX = e.changedTouches[0].screenX;
        touchEndY = e.changedTouches[0].screenY;
        handleSwipe();
    }, { passive: true });
}

// Save reading position to localStorage and server (if authenticated)
function saveReadingPosition() {
    if (!currentBook || !currentChapter || !selectedTranslation) return;
    
    const position = {
        translation: selectedTranslation,
        book: currentBook,
        chapter: currentChapter
    };
    
    // Save to localStorage for all users
    try {
        localStorage.setItem('bibleReadingPosition', JSON.stringify(position));
    } catch (e) {
        console.error('Failed to save to localStorage:', e);
    }
    
    // Save to server for authenticated users
    fetch('/api/save-reading-position', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(position)
    }).catch(err => {
        // Silently fail if user is not authenticated or server error
        console.debug('Could not save to server:', err);
    });
}

// Load saved reading position (returns position object or null)
async function loadSavedPosition() {
    try {
        // First, try to get from server if authenticated
        const response = await fetch('/api/reading-position');
        const data = await response.json();
        
        if (data.authenticated && data.hasPosition) {
            return {
                translation: data.translation,
                book: data.book,
                chapter: data.chapter,
                source: 'server'
            };
        }
    } catch (err) {
        console.debug('Could not load from server:', err);
    }
    
    // Fall back to localStorage
    try {
        const saved = localStorage.getItem('bibleReadingPosition');
        if (saved) {
            const position = JSON.parse(saved);
            return {
                ...position,
                source: 'localStorage'
            };
        }
    } catch (e) {
        console.error('Failed to load from localStorage:', e);
    }
    
    return null;
}

// Check authentication status
async function checkAuthStatus() {
    try {
        const response = await fetch('/api/user');
        const data = await response.json();
        
        const userMenu = document.getElementById('userMenu');
        const loginButton = document.getElementById('loginButton');
        const userName = document.getElementById('userName');
        const userEmail = document.getElementById('userEmail');
        const userMenuButton = document.getElementById('userMenuButton');
        const userDropdown = document.getElementById('userDropdown');
        
        if (data.authenticated) {
            // Show user menu, hide login button
            userMenu.classList.remove('hidden');
            loginButton.classList.add('hidden');
            userName.textContent = data.username;
            userEmail.textContent = data.email;
            currentUserId = data.id;
            
            // Setup dropdown toggle
            userMenuButton.addEventListener('click', (e) => {
                e.stopPropagation();
                userDropdown.classList.toggle('hidden');
            });
            
            // Close dropdown when clicking outside
            document.addEventListener('click', (e) => {
                if (!userMenu.contains(e.target)) {
                    userDropdown.classList.add('hidden');
                }
            });
            
            // Initialize notes functionality for authenticated users
            initializeNotes();
        } else {
            // Show login button, hide user menu
            userMenu.classList.add('hidden');
            loginButton.classList.remove('hidden');
        }
    } catch (error) {
        console.error('Failed to check auth status:', error);
        // On error, show login button
        document.getElementById('userMenu').classList.add('hidden');
        document.getElementById('loginButton').classList.remove('hidden');
    }
}

// ============ NOTES FUNCTIONALITY ============

let notesVisible = false;
let currentNoteType = 'personal'; // 'personal' or 'group'
let selectedGroupId = null;
let userGroups = [];
let currentUserId = null;
let notesPollingInterval = null;
let currentNotesData = null; // Cache of current notes for comparison
let openCommentsSections = new Set(); // Track which comment sections are open
let commentsData = new Map(); // Cache of comments data for comparison

// Initialize notes functionality
function initializeNotes() {
    const notesSection = document.getElementById('notesSection');
    const toggleNotesBtn = document.getElementById('toggleNotesBtn');
    const personalNotesTab = document.getElementById('personalNotesTab');
    const groupNotesTab = document.getElementById('groupNotesTab');
    const createPersonalNoteBtn = document.getElementById('createPersonalNoteBtn');
    const createGroupNoteBtn = document.getElementById('createGroupNoteBtn');
    const groupNoteSelector = document.getElementById('groupNoteSelector');
    
    // Show notes section and make notes visible by default for authenticated users
    notesSection.classList.remove('hidden');
    notesVisible = true;
    toggleNotesBtn.textContent = 'Hide Notes';
    document.getElementById('personalNotesView').classList.remove('hidden');
    
    // Start polling for authenticated users
    startNotesPolling();
    
    // Toggle notes visibility
    toggleNotesBtn.addEventListener('click', () => {
        notesVisible = !notesVisible;
        const personalNotesView = document.getElementById('personalNotesView');
        const groupNotesView = document.getElementById('groupNotesView');
        
        if (notesVisible) {
            personalNotesView.classList.remove('hidden');
            toggleNotesBtn.textContent = 'Hide Notes';
            if (currentBook && currentChapter) {
                loadNotes();
            }
            startNotesPolling();
        } else {
            personalNotesView.classList.add('hidden');
            groupNotesView.classList.add('hidden');
            toggleNotesBtn.textContent = 'Show Notes';
            stopNotesPolling();
        }
    });
    
    // Tab switching
    personalNotesTab.addEventListener('click', () => {
        currentNoteType = 'personal';
        personalNotesTab.classList.add('border-amber-600', 'text-amber-700');
        personalNotesTab.classList.remove('border-transparent', 'text-gray-600');
        groupNotesTab.classList.remove('border-amber-600', 'text-amber-700');
        groupNotesTab.classList.add('border-transparent', 'text-gray-600');
        document.getElementById('personalNotesView').classList.remove('hidden');
        document.getElementById('groupNotesView').classList.add('hidden');
        if (currentBook && currentChapter) loadPersonalNotes();
        restartNotesPolling();
    });
    
    groupNotesTab.addEventListener('click', () => {
        currentNoteType = 'group';
        groupNotesTab.classList.add('border-amber-600', 'text-amber-700');
        groupNotesTab.classList.remove('border-transparent', 'text-gray-600');
        personalNotesTab.classList.remove('border-amber-600', 'text-amber-700');
        personalNotesTab.classList.add('border-transparent', 'text-gray-600');
        document.getElementById('groupNotesView').classList.remove('hidden');
        document.getElementById('personalNotesView').classList.add('hidden');
        loadUserGroups();
        restartNotesPolling();
    });
    
    // Create personal note
    createPersonalNoteBtn.addEventListener('click', () => {
        showCreateNoteDialog('personal');
    });
    
    // Create group note
    createGroupNoteBtn.addEventListener('click', () => {
        if (selectedGroupId) {
            showCreateNoteDialog('group', selectedGroupId);
        }
    });
    
    // Group selector change
    groupNoteSelector.addEventListener('change', (e) => {
        selectedGroupId = e.target.value ? parseInt(e.target.value) : null;
        createGroupNoteBtn.disabled = !selectedGroupId;
        
        // Save to localStorage
        if (selectedGroupId) {
            localStorage.setItem('lastSelectedGroupId', selectedGroupId.toString());
        } else {
            localStorage.removeItem('lastSelectedGroupId');
        }
        
        if (selectedGroupId && currentBook && currentChapter) {
            loadGroupNotes(selectedGroupId);
        }
        restartNotesPolling();
    });
}

// Start polling for notes updates
function startNotesPolling() {
    stopNotesPolling(); // Clear any existing interval
    
    // Start polling immediately and then every 2 seconds
    pollNotesUpdates(); // First poll immediately
    
    notesPollingInterval = setInterval(() => {
        if (notesVisible && currentBook && currentChapter) {
            pollNotesUpdates();
        }
    }, 2000); // Poll every 2 seconds
    
    console.log('Started notes polling');
}

// Stop polling for notes updates
function stopNotesPolling() {
    if (notesPollingInterval) {
        clearInterval(notesPollingInterval);
        notesPollingInterval = null;
        console.log('Stopped notes polling');
    }
}

// Restart polling (when switching tabs or groups)
function restartNotesPolling() {
    if (notesVisible) {
        startNotesPolling();
    }
}

// Poll for notes updates
async function pollNotesUpdates() {
    try {
        let response;
        if (currentNoteType === 'personal') {
            response = await fetch(`/api/notes/personal?book=${encodeURIComponent(currentBook)}&chapter=${currentChapter}`);
        } else if (currentNoteType === 'group' && selectedGroupId) {
            response = await fetch(`/api/notes/group?group_id=${selectedGroupId}&book=${encodeURIComponent(currentBook)}&chapter=${currentChapter}`);
        } else {
            return;
        }
        
        if (!response.ok) return;
        const notes = await response.json();
        
        // Compare with current notes to see if update is needed
        const notesJson = JSON.stringify(notes);
        if (notesJson !== currentNotesData) {
            currentNotesData = notesJson;
            const containerId = currentNoteType === 'personal' ? 'personalNotesList' : 'groupNotesList';
            displayNotes(notes, containerId, currentUserId);
        }
        
        // Also poll open comments
        await pollOpenComments();
    } catch (error) {
        console.debug('Polling error:', error);
    }
}

// Poll for comments updates
async function pollOpenComments() {
    for (const noteId of openCommentsSections) {
        try {
            const response = await fetch(`/api/notes/${noteId}/comments`);
            if (!response.ok) continue;
            const comments = await response.json();
            
            const commentsJson = JSON.stringify(comments);
            const cachedData = commentsData.get(noteId);
            
            if (commentsJson !== cachedData) {
                commentsData.set(noteId, commentsJson);
                await updateCommentsDisplay(noteId, comments);
            }
        } catch (error) {
            console.debug('Error polling comments:', error);
        }
    }
}

// Load notes (called when chapter changes)
async function loadNotes() {
    if (!notesVisible) return;
    
    currentNotesData = null; // Reset cache when loading new chapter
    openCommentsSections.clear(); // Clear open comments when changing chapters
    commentsData.clear();
    if (currentNoteType === 'personal') {
        await loadPersonalNotes();
    } else if (currentNoteType === 'group' && selectedGroupId) {
        await loadGroupNotes(selectedGroupId);
    }
    restartNotesPolling();
}

// Load personal notes
async function loadPersonalNotes() {
    if (!currentBook || !currentChapter) return;
    
    try {
        const response = await fetch(`/api/notes/personal?book=${encodeURIComponent(currentBook)}&chapter=${currentChapter}`);
        if (!response.ok) throw new Error('Failed to load notes');
        const notes = await response.json();
        currentNotesData = JSON.stringify(notes);
        displayNotes(notes, 'personalNotesList', currentUserId);
    } catch (error) {
        console.error('Error loading personal notes:', error);
    }
}

// Load group notes
async function loadGroupNotes(groupId) {
    if (!currentBook || !currentChapter) return;
    
    try {
        const response = await fetch(`/api/notes/group?group_id=${groupId}&book=${encodeURIComponent(currentBook)}&chapter=${currentChapter}`);
        if (!response.ok) throw new Error('Failed to load notes');
        const notes = await response.json();
        currentNotesData = JSON.stringify(notes);
        displayNotes(notes, 'groupNotesList', currentUserId);
    } catch (error) {
        console.error('Error loading group notes:', error);
    }
}

// Load user's groups for the dropdown
async function loadUserGroups() {
    try {
        const response = await fetch('/api/groups/list');
        if (!response.ok) throw new Error('Failed to load groups');
        userGroups = await response.json();
        
        const selector = document.getElementById('groupNoteSelector');
        selector.innerHTML = '<option value="">Select a study group...</option>' +
            userGroups.map(g => `<option value="${g.id}">${escapeHtml(g.name)}</option>`).join('');
        
        // Restore last selected group from localStorage
        const lastGroupId = localStorage.getItem('lastSelectedGroupId');
        if (lastGroupId && userGroups.some(g => g.id.toString() === lastGroupId)) {
            selector.value = lastGroupId;
            selectedGroupId = parseInt(lastGroupId);
            createGroupNoteBtn.disabled = false;
            if (currentBook && currentChapter) {
                loadGroupNotes(selectedGroupId);
            }
        }
    } catch (error) {
        console.error('Error loading groups:', error);
    }
}

// Display notes in the list
function displayNotes(notes, containerId, currentUserId) {
    const container = document.getElementById(containerId);
    
    if (!notes || notes.length === 0) {
        container.innerHTML = '<p class="text-gray-500 text-sm italic">No notes yet for this chapter.</p>';
        return;
    }
    
    container.innerHTML = notes.map(note => {
        // Check if current user owns this note
        const canEdit = currentUserId && note.user_id === currentUserId;
        // Check if note is temporary (not yet saved to server)
        const isTemp = String(note.id).startsWith('temp-');
        // Escape content for use in onclick attribute - replace backticks and backslashes
        const escapedContent = escapeHtml(note.content).replace(/\\/g, '\\\\').replace(/`/g, '\\`');
        
        return `
        <div class="bg-white rounded-lg p-4 shadow-sm border border-gray-200 ${isTemp ? 'opacity-70' : ''}" id="note-${note.id}">
            <div class="flex items-start justify-between mb-2">
                <div class="flex-1">
                    <p class="text-sm font-medium text-gray-700">
                        ${escapeHtml(note.username)}
                        ${isTemp ? '<span class="ml-2 text-xs text-amber-600">⏳ Saving...</span>' : ''}
                    </p>
                    <p class="text-xs text-gray-500">${new Date(note.created_at).toLocaleString()}</p>
                </div>
                ${canEdit && !isTemp ? `
                    <div class="flex gap-2">
                        <button onclick="editNote('${note.id}', \`${escapedContent}\`)"
                                class="text-blue-600 hover:text-blue-800 text-xs">Edit</button>
                        <button onclick="deleteNote('${note.id}')"
                                class="text-red-600 hover:text-red-800 text-xs">Delete</button>
                    </div>
                ` : ''}
            </div>
            <p class="text-gray-800 whitespace-pre-wrap">${escapeHtml(note.content)}</p>
            <div class="mt-3 pt-3 border-t border-gray-100">
                <button onclick="toggleComments('${note.id}')" 
                        class="text-sm text-blue-600 hover:text-blue-800" ${isTemp ? 'disabled' : ''}>
                    💬 Comments
                </button>
                <div id="comments-${note.id}" class="hidden mt-3 space-y-2">
                    <!-- Comments will be loaded here -->
                </div>
            </div>
        </div>
        `;
    }).join('');
}

// Modal helper functions
function showNoteEditor(title, initialContent, onSave) {
    const modal = document.getElementById('noteEditorModal');
    const titleEl = document.getElementById('noteEditorTitle');
    const contentEl = document.getElementById('noteEditorContent');
    const saveBtn = document.getElementById('noteEditorSave');
    const cancelBtn = document.getElementById('noteEditorCancel');
    
    titleEl.textContent = title;
    contentEl.value = initialContent || '';
    modal.classList.remove('hidden');
    modal.classList.add('flex');
    contentEl.focus();
    
    const handleSave = () => {
        const content = contentEl.value.trim();
        if (content) {
            onSave(content);
        }
        closeNoteEditor();
    };
    
    const handleCancel = () => {
        closeNoteEditor();
    };
    
    const closeNoteEditor = () => {
        modal.classList.add('hidden');
        modal.classList.remove('flex');
        saveBtn.removeEventListener('click', handleSave);
        cancelBtn.removeEventListener('click', handleCancel);
    };
    
    saveBtn.addEventListener('click', handleSave);
    cancelBtn.addEventListener('click', handleCancel);
}

function showDeleteConfirm(onConfirm) {
    const modal = document.getElementById('deleteConfirmModal');
    const yesBtn = document.getElementById('deleteConfirmYes');
    const noBtn = document.getElementById('deleteConfirmNo');
    
    modal.classList.remove('hidden');
    modal.classList.add('flex');
    
    const handleYes = () => {
        onConfirm();
        closeDeleteConfirm();
    };
    
    const handleNo = () => {
        closeDeleteConfirm();
    };
    
    const closeDeleteConfirm = () => {
        modal.classList.add('hidden');
        modal.classList.remove('flex');
        yesBtn.removeEventListener('click', handleYes);
        noBtn.removeEventListener('click', handleNo);
    };
    
    yesBtn.addEventListener('click', handleYes);
    noBtn.addEventListener('click', handleNo);
}

function showNoteMessage(title, message) {
    const modal = document.getElementById('noteMessageModal');
    const titleEl = document.getElementById('noteMessageTitle');
    const messageEl = document.getElementById('noteMessageText');
    const okBtn = document.getElementById('noteMessageOk');
    
    titleEl.textContent = title;
    messageEl.textContent = message;
    modal.classList.remove('hidden');
    modal.classList.add('flex');
    
    const handleOk = () => {
        modal.classList.add('hidden');
        modal.classList.remove('flex');
        okBtn.removeEventListener('click', handleOk);
    };
    
    okBtn.addEventListener('click', handleOk);
}

// Show create note dialog
function showCreateNoteDialog(type, groupId = null) {
    showNoteEditor('Create Note', '', (content) => {
        createNote(content, type, groupId);
    });
}

// Create a note
async function createNote(content, type, groupId = null) {
    if (!currentBook || !currentChapter) return;
    
    // Optimistic update - add note to UI immediately
    const optimisticNote = {
        id: 'temp-' + Date.now(),
        user_id: currentUserId,
        username: document.getElementById('userName').textContent,
        content: content,
        created_at: new Date().toISOString(),
        book: currentBook,
        chapter: currentChapter,
        group_id: groupId
    };
    
    addNoteToUI(optimisticNote);
    
    try {
        const response = await fetch('/api/notes/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                book: currentBook,
                chapter: currentChapter,
                content: content,
                group_id: groupId
            })
        });
        
        if (!response.ok) {
            const errorText = await response.text();
            removeNoteFromUI(optimisticNote.id);
            throw new Error(errorText || 'Failed to create note');
        }
        
        // Get the real note from server response
        const realNote = await response.json();
        
        // Replace temp note with real note in UI
        replaceNoteInUI(optimisticNote.id, realNote);
        
        // Update cache to include the new note
        currentNotesData = null;
    } catch (error) {
        console.error('Error creating note:', error);
        showNoteMessage('Error', 'Failed to create note: ' + error.message);
    }
}

// Edit a note
async function editNote(noteId, currentContent) {
    // Prevent editing temporary notes
    if (String(noteId).startsWith('temp-')) {
        showNoteMessage('Please Wait', 'This note is still being saved. Please wait a moment and try again.');
        return;
    }
    
    showNoteEditor('Edit Note', currentContent, async (content) => {
        if (content === currentContent) return;
        
        // Optimistic update - update UI immediately
        updateNoteContentInUI(noteId, content);
        
        try {
            const response = await fetch(`/api/notes/${noteId}/update`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ content: content })
            });
            
            if (!response.ok) {
                // Revert on error
                updateNoteContentInUI(noteId, currentContent);
                throw new Error('Failed to update note');
            }
            
            // Force refresh to get updated timestamp
            currentNotesData = null;
        } catch (error) {
            console.error('Error updating note:', error);
            showNoteMessage('Error', 'Failed to update note');
        }
    });
}

// Delete a note
async function deleteNote(noteId) {
    // Prevent deleting temporary notes
    if (String(noteId).startsWith('temp-')) {
        showNoteMessage('Please Wait', 'This note is still being saved. Please wait a moment and try again.');
        return;
    }
    
    showDeleteConfirm(async () => {
        // Optimistic update - remove from UI immediately
        removeNoteFromUI(noteId);
        
        try {
            const response = await fetch(`/api/notes/${noteId}/delete`, { method: 'DELETE' });
            if (!response.ok) {
                // On error, force reload to restore
                currentNotesData = null;
                await loadNotes();
                throw new Error('Failed to delete note');
            }
            
            // Update cache
            currentNotesData = null;
        } catch (error) {
            console.error('Error deleting note:', error);
            showNoteMessage('Error', 'Failed to delete note');
        }
    });
}

// Toggle comments visibility
async function toggleComments(noteId) {
    // Prevent opening comments on temporary notes
    if (String(noteId).startsWith('temp-')) {
        return;
    }
    
    const commentsDiv = document.getElementById(`comments-${noteId}`);
    
    if (commentsDiv.classList.contains('hidden')) {
        commentsDiv.classList.remove('hidden');
        openCommentsSections.add(noteId);
        await loadComments(noteId);
    } else {
        commentsDiv.classList.add('hidden');
        openCommentsSections.delete(noteId);
        commentsData.delete(noteId);
    }
}

// Load comments for a note
async function loadComments(noteId) {
    try {
        const response = await fetch(`/api/notes/${noteId}/comments`);
        if (!response.ok) throw new Error('Failed to load comments');
        const comments = await response.json();
        
        commentsData.set(noteId, JSON.stringify(comments));
        await updateCommentsDisplay(noteId, comments);
    } catch (error) {
        console.error('Error loading comments:', error);
    }
}

// Update comments display
async function updateCommentsDisplay(noteId, comments) {
    const commentsDiv = document.getElementById(`comments-${noteId}`);
    if (!commentsDiv) return;
    
    commentsDiv.innerHTML = `
        <div class="bg-gray-50 rounded p-3 space-y-2">
            ${comments.length === 0 ? '<p class="text-gray-500 text-xs italic">No comments yet.</p>' : ''}
            ${comments.map(c => `
                <div class="bg-white rounded p-2 text-sm" id="comment-${c.id}">
                    <div class="flex items-start justify-between mb-1">
                        <span class="font-medium text-xs text-gray-700">${escapeHtml(c.username)}</span>
                        <span class="text-xs text-gray-500">${new Date(c.created_at).toLocaleString()}</span>
                    </div>
                    <p class="text-gray-800 text-xs">${escapeHtml(c.content)}</p>
                </div>
            `).join('')}
            <div class="mt-2">
                <button onclick="addComment(${noteId})" 
                        class="text-xs text-blue-600 hover:text-blue-800">
                    + Add Comment
                </button>
            </div>
        </div>
    `;
}

// Add a comment
async function addComment(noteId) {
    showNoteEditor('Add Comment', '', async (content) => {
        // Optimistic update - add comment to UI immediately
        const optimisticComment = {
            id: 'temp-' + Date.now(),
            note_id: noteId,
            user_id: currentUserId,
            username: document.getElementById('userName').textContent,
            content: content,
            created_at: new Date().toISOString()
        };
        
        await addCommentToUI(noteId, optimisticComment);
        
        try {
            const response = await fetch('/api/comments/create', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    note_id: noteId,
                    content: content
                })
            });
            
            if (!response.ok) {
                // Revert on error
                commentsData.delete(noteId);
                await loadComments(noteId);
                throw new Error('Failed to create comment');
            }
            
            // Force refresh to get real comment
            commentsData.delete(noteId);
            await loadComments(noteId);
        } catch (error) {
            console.error('Error creating comment:', error);
            showNoteMessage('Error', 'Failed to create comment');
        }
    });
}

// Optimistic UI update helpers
function addNoteToUI(note) {
    const containerId = currentNoteType === 'personal' ? 'personalNotesList' : 'groupNotesList';
    const container = document.getElementById(containerId);
    
    const canEdit = currentUserId && note.user_id === currentUserId;
    // Check if note is temporary (not yet saved to server)
    const isTemp = String(note.id).startsWith('temp-');
    // Escape content for use in onclick attribute - replace backticks and backslashes
    const escapedContent = escapeHtml(note.content).replace(/\\/g, '\\\\').replace(/`/g, '\\`');
    
    const noteDiv = document.createElement('div');
    noteDiv.className = `bg-white rounded-lg p-4 shadow-sm border border-gray-200 ${isTemp ? 'opacity-70' : ''}`;
    noteDiv.id = `note-${note.id}`;
    noteDiv.innerHTML = `
        <div class="flex items-start justify-between mb-2">
            <div class="flex-1">
                <p class="text-sm font-medium text-gray-700">
                    ${escapeHtml(note.username)}
                    ${isTemp ? '<span class="ml-2 text-xs text-amber-600">⏳ Saving...</span>' : ''}
                </p>
                <p class="text-xs text-gray-500">${new Date(note.created_at).toLocaleString()}</p>
            </div>
            ${canEdit && !isTemp ? `
                <div class="flex gap-2">
                    <button onclick="editNote('${note.id}', \`${escapedContent}\`)"
                            class="text-blue-600 hover:text-blue-800 text-xs">Edit</button>
                    <button onclick="deleteNote('${note.id}')"
                            class="text-red-600 hover:text-red-800 text-xs">Delete</button>
                </div>
            ` : ''}
        </div>
        <p class="text-gray-800 whitespace-pre-wrap">${escapeHtml(note.content)}</p>
        <div class="mt-3 pt-3 border-t border-gray-100">
            <button onclick="toggleComments('${note.id}')" 
                    class="text-sm text-blue-600 hover:text-blue-800" ${isTemp ? 'disabled' : ''}>
                💬 Comments
            </button>
            <div id="comments-${note.id}" class="hidden mt-3 space-y-2">
                <!-- Comments will be loaded here -->
            </div>
        </div>
    `;
    
    // Add to top of list
    if (container.firstChild && container.firstChild.className !== 'text-gray-500') {
        container.insertBefore(noteDiv, container.firstChild);
    } else {
        // Replace "no notes" message if it exists
        container.innerHTML = '';
        container.appendChild(noteDiv);
    }
}

function removeNoteFromUI(noteId) {
    const noteElement = document.getElementById(`note-${noteId}`);
    if (noteElement) {
        noteElement.remove();
        
        // Check if container is now empty
        const containerId = currentNoteType === 'personal' ? 'personalNotesList' : 'groupNotesList';
        const container = document.getElementById(containerId);
        if (container.children.length === 0) {
            container.innerHTML = '<p class="text-gray-500 text-sm italic">No notes yet for this chapter.</p>';
        }
    }
}

function replaceNoteInUI(tempId, realNote) {
    const tempElement = document.getElementById(`note-${tempId}`);
    if (!tempElement) return;
    
    const canEdit = currentUserId && realNote.user_id === currentUserId;
    const escapedContent = escapeHtml(realNote.content).replace(/\\/g, '\\\\').replace(/`/g, '\\`');
    
    // Create the replacement note element
    const noteDiv = document.createElement('div');
    noteDiv.className = 'bg-white rounded-lg p-4 shadow-sm border border-gray-200';
    noteDiv.id = `note-${realNote.id}`;
    noteDiv.innerHTML = `
        <div class="flex items-start justify-between mb-2">
            <div class="flex-1">
                <p class="text-sm font-medium text-gray-700">${escapeHtml(realNote.username)}</p>
                <p class="text-xs text-gray-500">${new Date(realNote.created_at).toLocaleString()}</p>
            </div>
            ${canEdit ? `
                <div class="flex gap-2">
                    <button onclick="editNote('${realNote.id}', \`${escapedContent}\`)"
                            class="text-blue-600 hover:text-blue-800 text-xs">Edit</button>
                    <button onclick="deleteNote('${realNote.id}')"
                            class="text-red-600 hover:text-red-800 text-xs">Delete</button>
                </div>
            ` : ''}
        </div>
        <p class="text-gray-800 whitespace-pre-wrap">${escapeHtml(realNote.content)}</p>
        <div class="mt-3 pt-3 border-t border-gray-100">
            <button onclick="toggleComments('${realNote.id}')" 
                    class="text-sm text-blue-600 hover:text-blue-800">
                💬 Comments
            </button>
            <div id="comments-${realNote.id}" class="hidden mt-3 space-y-2">
                <!-- Comments will be loaded here -->
            </div>
        </div>
    `;
    
    // Replace the temp element with the real one
    tempElement.replaceWith(noteDiv);
}

function updateNoteContentInUI(noteId, newContent) {
    const noteElement = document.getElementById(`note-${noteId}`);
    if (noteElement) {
        const contentP = noteElement.querySelector('.whitespace-pre-wrap');
        if (contentP) {
            contentP.textContent = newContent;
        }
        
        // Update the edit button's onclick to use new content
        const editBtn = noteElement.querySelector('button[onclick*="editNote"]');
        if (editBtn) {
            const escapedContent = escapeHtml(newContent).replace(/\\/g, '\\\\').replace(/`/g, '\\`');
            editBtn.setAttribute('onclick', `editNote('${noteId}', \`${escapedContent}\`)`);
        }
    }
}

async function addCommentToUI(noteId, comment) {
    const commentsDiv = document.getElementById(`comments-${noteId}`);
    if (!commentsDiv) return;
    
    // If comments section is open, add the comment
    if (!commentsDiv.classList.contains('hidden')) {
        const commentDiv = document.createElement('div');
        commentDiv.className = 'bg-white rounded p-2 text-sm';
        commentDiv.id = `comment-${comment.id}`;
        commentDiv.innerHTML = `
            <div class="flex items-start justify-between mb-1">
                <span class="font-medium text-xs text-gray-700">${escapeHtml(comment.username)}</span>
                <span class="text-xs text-gray-500">${new Date(comment.created_at).toLocaleString()}</span>
            </div>
            <p class="text-gray-800 text-xs">${escapeHtml(comment.content)}</p>
        `;
        
        const commentsContainer = commentsDiv.querySelector('.bg-gray-50');
        if (commentsContainer) {
            // Remove "no comments" message if it exists
            const noCommentsMsg = commentsContainer.querySelector('.text-gray-500.italic');
            if (noCommentsMsg) {
                noCommentsMsg.remove();
            }
            
            // Add before the "Add Comment" button
            const addBtn = commentsContainer.querySelector('.mt-2');
            if (addBtn) {
                commentsContainer.insertBefore(commentDiv, addBtn);
            }
        }
    }
}

// Utility function for HTML escaping
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Start the application
init();

