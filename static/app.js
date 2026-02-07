// State management
let bibleBooks = [];
let currentBook = null;
let currentChapter = 1;
let maxChapter = 1;
let showReferences = false; // Toggle for showing/hiding references
let showVerseComments = false; // Toggle for showing/hiding verse comments
let selectedCommentGroup = null; // null for personal, groupID for group comments
let userGroups = []; // Available study groups for the user
let selectedTranslation = ''; // Currently selected translation
let translationInfo = {}; // Translation metadata (fullName, description)
let sidebarVisible = true; // Sidebar visibility state
let currentUserId = null; // Current authenticated user ID

// Load UI preferences from localStorage
function loadUIPreferences() {
    try {
        const prefs = localStorage.getItem('uiPreferences');
        if (prefs) {
            const parsed = JSON.parse(prefs);
            showReferences = parsed.showReferences !== undefined ? parsed.showReferences : false;
            showVerseComments = parsed.showVerseComments !== undefined ? parsed.showVerseComments : false;
            selectedCommentGroup = parsed.selectedCommentGroup !== undefined ? parsed.selectedCommentGroup : null;
        }
    } catch (error) {
        console.error('Failed to load UI preferences:', error);
    }
}

// Save UI preferences to localStorage
function saveUIPreferences() {
    try {
        localStorage.setItem('uiPreferences', JSON.stringify({
            showReferences,
            showVerseComments,
            selectedCommentGroup
        }));
    } catch (error) {
        console.error('Failed to save UI preferences:', error);
    }
}

// DOM Elements
const bookList = document.getElementById('bookList');
const chapterTitle = document.getElementById('chapterTitle');
const versesContainer = document.getElementById('versesContainer');
const prevChapterBtn = document.getElementById('prevChapter');
const nextChapterBtn = document.getElementById('nextChapter');
const chapterSelector = document.getElementById('chapterSelector');
const chapterSelect = document.getElementById('chapterSelect');
const toggleReferencesBtn = document.getElementById('toggleReferences');
const toggleVerseCommentsBtn = document.getElementById('toggleVerseComments');
const verseCommentsGroupSelect = document.getElementById('verseCommentsGroupSelect');
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
    
    // Load UI preferences from localStorage
    loadUIPreferences();
    
    // Connect to WebSocket for real-time updates
    connectWebSocket();
    
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
        await renderVerses(data.verses);
        
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
async function renderVerses(verses) {
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
    if (!showVerseComments) {
        container.classList.add('hide-verse-comments');
    }
    
    // Load all verse comments if enabled
    const verseCommentsMap = new Map();
    if (showVerseComments && currentUserId) {
        const commentPromises = verses.map(v => loadVerseComments(currentBook, currentChapter, v.verse));
        const allComments = await Promise.all(commentPromises);
        verses.forEach((v, idx) => {
            verseCommentsMap.set(v.verse, allComments[idx] || []);
        });
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
        
        // Add inline + button for verse comments if enabled and no comments exist
        if (showVerseComments && verseCommentsMap.has(verseNum)) {
            const comments = verseCommentsMap.get(verseNum);
            const commentCount = countTotalComments(comments);
            if (commentCount === 0) {
                html += `<button class="verse-comment-add-btn" onclick="showAddCommentForm('${escapeHtml(currentBook)}', ${currentChapter}, ${verseNum})" 
                        style="padding: 0.125rem 0.25rem; background-color: #9333ea; color: white; border-radius: 0.25rem; font-size: 0.7rem; cursor: pointer; border: none; margin-left: 0.25rem; vertical-align: middle;"
                        title="Add a note to this verse">
                    <svg style="width: 7px; height: 7px; display: inline;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M12 4v16m8-8H4"></path>
                    </svg>
                </button>`;
            }
        }
        
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
        
        // Add verse comments if they exist and should be shown
        if (showVerseComments && verseCommentsMap.has(verseNum)) {
            const comments = verseCommentsMap.get(verseNum);
            html += renderVerseCommentsSection(currentBook, currentChapter, verseNum, comments);
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

// Helper function to generate profile picture HTML
function getProfilePictureHTML(user, size = 'w-8 h-8') {
    if (user.profile_picture_url) {
        return `<img src="${escapeHtml(user.profile_picture_url)}" class="${size} rounded-full object-cover flex-shrink-0" alt="Profile">`;
    }
    
    const initials = user.first_name && user.last_name 
        ? (user.first_name[0] + user.last_name[0]).toUpperCase() 
        : user.username[0].toUpperCase();
    
    return `<div class="${size} rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white text-xs font-bold flex-shrink-0">${initials}</div>`;
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
        // Set initial button text based on restored state
        toggleReferencesBtn.innerHTML = showReferences 
            ? '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"></path></svg> Hide References'
            : '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21"></path></svg> Show References';
        
        toggleReferencesBtn.addEventListener('click', () => {
            showReferences = !showReferences;
            toggleReferencesBtn.innerHTML = showReferences 
                ? '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"></path></svg> Hide References'
                : '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21"></path></svg> Show References';
            saveUIPreferences(); // Save preference
            loadChapter(); // Reload to apply change
        });
    }
    
    // Toggle verse comments button
    if (toggleVerseCommentsBtn) {
        // Set initial button text based on restored state
        toggleVerseCommentsBtn.innerHTML = showVerseComments 
            ? '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"></path></svg> <span class="hidden sm:inline">Hide Notes</span>'
            : '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"></path></svg> <span class="hidden sm:inline">Show Notes</span>';
        
        toggleVerseCommentsBtn.addEventListener('click', () => {
            showVerseComments = !showVerseComments;
            toggleVerseCommentsBtn.innerHTML = showVerseComments 
                ? '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"></path></svg> <span class="hidden sm:inline">Hide Notes</span>'
                : '<svg class="w-5 h-5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"></path></svg> <span class="hidden sm:inline">Show Notes</span>';
            saveUIPreferences(); // Save preference
            loadChapter(); // Reload to apply change
        });
    }
    
    // Verse comments group selector
    if (verseCommentsGroupSelect) {
        // Restore selected group from preferences
        if (selectedCommentGroup !== null) {
            verseCommentsGroupSelect.value = selectedCommentGroup;
        }
        
        verseCommentsGroupSelect.addEventListener('change', (e) => {
            selectedCommentGroup = e.target.value ? parseInt(e.target.value) : null;
            saveUIPreferences(); // Save preference
            if (showVerseComments) {
                loadChapter(); // Reload to load comments for selected group
            }
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
        const communityMenu = document.getElementById('communityMenu');
        const communityMenuButton = document.getElementById('communityMenuButton');
        const communityDropdown = document.getElementById('communityDropdown');
        
        if (data.authenticated) {
            // Show user menu and community menu, hide login button
            userMenu.classList.remove('hidden');
            communityMenu.classList.remove('hidden');
            loginButton.classList.add('hidden');
            
            // Display first + last name instead of username
            const displayName = data.first_name && data.last_name 
                ? `${data.first_name} ${data.last_name}` 
                : data.username;
            userName.textContent = displayName;
            userEmail.textContent = data.email;
            currentUserId = data.id;
            
            // Update profile picture in user menu
            const userPicDiv = document.getElementById('userProfilePic');
            if (userPicDiv) {
                userPicDiv.innerHTML = getProfilePictureHTML(data, 'w-8 h-8');
            }
            
            // Setup user dropdown toggle
            userMenuButton.addEventListener('click', (e) => {
                e.stopPropagation();
                userDropdown.classList.toggle('hidden');
                communityDropdown.classList.add('hidden'); // Close community dropdown
            });
            
            // Setup community dropdown toggle
            communityMenuButton.addEventListener('click', (e) => {
                e.stopPropagation();
                communityDropdown.classList.toggle('hidden');
                userDropdown.classList.add('hidden'); // Close user dropdown
            });
            
            // Close dropdowns when clicking outside
            document.addEventListener('click', (e) => {
                if (!userMenu.contains(e.target)) {
                    userDropdown.classList.add('hidden');
                }
                if (!communityMenu.contains(e.target)) {
                    communityDropdown.classList.add('hidden');
                }
            });
            
            // Load user groups for verse comments
            loadUserGroupsForComments();
        } else {
            // Show login button, hide user menu and community menu
            userMenu.classList.add('hidden');
            communityMenu.classList.add('hidden');
            loginButton.classList.remove('hidden');
        }
    } catch (error) {
        console.error('Failed to check auth status:', error);
        // On error, show login button
        document.getElementById('userMenu').classList.add('hidden');
        document.getElementById('communityMenu').classList.add('hidden');
        document.getElementById('loginButton').classList.remove('hidden');
    }
}

// ============ VERSE COMMENTS (INLINE NOTES) ============

// Load user groups for verse comments dropdown
async function loadUserGroupsForComments() {
    const createPersonalNoteBtn = document.getElementById('createPersonalNoteBtn');
    const createGroupNoteBtn = document.getElementById('createGroupNoteBtn');
    const groupNoteSelector = document.getElementById('groupNoteSelector');
    
    // Show notes section and apply saved visibility state
    notesSection.classList.remove('hidden');
    toggleNotesBtn.textContent = notesVisible ? 'Hide Notes' : 'Show Notes';
    
    // Apply saved note type (personal vs group)
    if (currentNoteType === 'group') {
        groupNotesTab.classList.add('border-amber-600', 'text-amber-700');
        groupNotesTab.classList.remove('border-transparent', 'text-gray-600');
        personalNotesTab.classList.remove('border-amber-600', 'text-amber-700');
        personalNotesTab.classList.add('border-transparent', 'text-gray-600');
        document.getElementById('groupNotesView').classList.remove('hidden');
        document.getElementById('personalNotesView').classList.add('hidden');
    } else {
        personalNotesTab.classList.add('border-amber-600', 'text-amber-700');
        personalNotesTab.classList.remove('border-transparent', 'text-gray-600');
        groupNotesTab.classList.remove('border-amber-600', 'text-amber-700');
        groupNotesTab.classList.add('border-transparent', 'text-gray-600');
        document.getElementById('personalNotesView').classList.remove('hidden');
        document.getElementById('groupNotesView').classList.add('hidden');
    }
    
    // Apply visibility state to views and enable/disable controls
    if (notesVisible) {
        if (currentNoteType === 'personal') {
            document.getElementById('personalNotesView').classList.remove('hidden');
            // Disable group selector when on personal tab
            groupNoteSelector.disabled = true;
        } else {
            document.getElementById('groupNotesView').classList.remove('hidden');
            // Enable group selector when on group tab
            groupNoteSelector.disabled = false;
            // Load groups when restoring group notes state
            loadUserGroups();
        }
        // Enable tabs
        personalNotesTab.disabled = false;
        groupNotesTab.disabled = false;
        personalNotesTab.classList.remove('opacity-50', 'cursor-not-allowed');
        groupNotesTab.classList.remove('opacity-50', 'cursor-not-allowed');
        startNotesPolling();
    } else {
        document.getElementById('personalNotesView').classList.add('hidden');
        document.getElementById('groupNotesView').classList.add('hidden');
        // Disable tabs
        personalNotesTab.disabled = true;
        groupNotesTab.disabled = true;
        personalNotesTab.classList.add('opacity-50', 'cursor-not-allowed');
        groupNotesTab.classList.add('opacity-50', 'cursor-not-allowed');
        // Disable group selector
        groupNoteSelector.disabled = true;
    }
    
    // Toggle notes visibility
    toggleNotesBtn.addEventListener('click', () => {
        notesVisible = !notesVisible;
        const personalNotesView = document.getElementById('personalNotesView');
        const groupNotesView = document.getElementById('groupNotesView');
        
        if (notesVisible) {
            if (currentNoteType === 'personal') {
                personalNotesView.classList.remove('hidden');
            } else {
                groupNotesView.classList.remove('hidden');
            }
            toggleNotesBtn.textContent = 'Hide Notes';
            // Enable tabs
            personalNotesTab.disabled = false;
            groupNotesTab.disabled = false;
            personalNotesTab.classList.remove('opacity-50', 'cursor-not-allowed');
            groupNotesTab.classList.remove('opacity-50', 'cursor-not-allowed');
            // Enable group selector if on group tab
            if (currentNoteType === 'group') {
                groupNoteSelector.disabled = false;
            }
            if (currentBook && currentChapter) {
                loadNotes();
            }
            startNotesPolling();
        } else {
            personalNotesView.classList.add('hidden');
            groupNotesView.classList.add('hidden');
            toggleNotesBtn.textContent = 'Show Notes';
            // Disable tabs
            personalNotesTab.disabled = true;
            groupNotesTab.disabled = true;
            personalNotesTab.classList.add('opacity-50', 'cursor-not-allowed');
            groupNotesTab.classList.add('opacity-50', 'cursor-not-allowed');
            // Disable group selector
            groupNoteSelector.disabled = true;
            stopNotesPolling();
        }
        saveUIPreferences(); // Save preference
    });
    
    // Tab switching
    personalNotesTab.addEventListener('click', () => {
        if (personalNotesTab.disabled) return;
        currentNoteType = 'personal';
        personalNotesTab.classList.add('border-amber-600', 'text-amber-700');
        personalNotesTab.classList.remove('border-transparent', 'text-gray-600');
        groupNotesTab.classList.remove('border-amber-600', 'text-amber-700');
        groupNotesTab.classList.add('border-transparent', 'text-gray-600');
        document.getElementById('personalNotesView').classList.remove('hidden');
        document.getElementById('groupNotesView').classList.add('hidden');
        // Disable group selector when on personal tab
        groupNoteSelector.disabled = true;
        if (currentBook && currentChapter) loadPersonalNotes();
        saveUIPreferences(); // Save preference
        restartNotesPolling();
    });
    
    groupNotesTab.addEventListener('click', () => {
        if (groupNotesTab.disabled) return;
        currentNoteType = 'group';
        groupNotesTab.classList.add('border-amber-600', 'text-amber-700');
        groupNotesTab.classList.remove('border-transparent', 'text-gray-600');
        personalNotesTab.classList.remove('border-amber-600', 'text-amber-700');
        personalNotesTab.classList.add('border-transparent', 'text-gray-600');
        document.getElementById('groupNotesView').classList.remove('hidden');
        document.getElementById('personalNotesView').classList.add('hidden');
        // Enable group selector when on group tab
        groupNoteSelector.disabled = false;
        loadUserGroups();
        saveUIPreferences(); // Save preference
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
        
        // Save to unified preferences
        saveUIPreferences();
        
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
        
        // Restore selected group from unified preferences
        if (selectedGroupId && userGroups.some(g => g.id === selectedGroupId)) {
            selector.value = selectedGroupId.toString();
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
                <div class="flex items-start gap-2 flex-1">
                    ${getProfilePictureHTML(note, 'w-8 h-8')}
                    <div class="flex-1 min-w-0">
                        <p class="text-sm font-medium text-gray-700">
                            ${note.first_name && note.last_name ? escapeHtml(`${note.first_name} ${note.last_name}`) : escapeHtml(note.username)}
                            ${isTemp ? '<span class="ml-2 text-xs text-amber-600">⏳ Saving...</span>' : ''}
                        </p>
                        <p class="text-xs text-gray-500">${new Date(note.created_at).toLocaleString()}</p>
                    </div>
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
            <div id="reactions-note-${note.id}" class="flex flex-wrap gap-1 items-center mt-2">
                ${isTemp ? '' : '<span class="text-xs text-gray-400">Loading reactions...</span>'}
            </div>
            <div class="mt-3 pt-3 border-t border-gray-100" id="comments-section-${note.id}">
                <!-- Comments will be loaded here automatically -->
            </div>
        </div>
        `;
    }).join('');
    
    // Load reactions for each note after rendering
    if (notes && notes.length > 0) {
        setTimeout(() => {
            notes.forEach(note => {
                if (!String(note.id).startsWith('temp-')) {
                    loadReactions('note', note.id);
                    // Load comments automatically for each note
                    loadNoteCommentsInline(note.id);
                }
            });
        }, 0);
    }
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

// These old functions have been replaced by the threaded comment system below
// (loadNoteCommentsInline, renderNoteCommentsSection, renderNoteComment, etc.)

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
            <div class="flex items-start gap-2 flex-1">
                ${getProfilePictureHTML(note, 'w-8 h-8')}
                <div class="flex-1 min-w-0">
                    <p class="text-sm font-medium text-gray-700">
                        ${note.first_name && note.last_name ? escapeHtml(`${note.first_name} ${note.last_name}`) : escapeHtml(note.username)}
                        ${isTemp ? '<span class="ml-2 text-xs text-amber-600">⏳ Saving...</span>' : ''}
                    </p>
                    <p class="text-xs text-gray-500">${new Date(note.created_at).toLocaleString()}</p>
                </div>
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
        ${isTemp ? '' : `<div class="mt-3 pt-3 border-t border-gray-100" id="comments-section-${note.id}">
            <!-- Comments will be loaded here automatically -->
        </div>`}
    `;
    
    // Add to top of list
    if (container.firstChild && container.firstChild.className !== 'text-gray-500') {
        container.insertBefore(noteDiv, container.firstChild);
    } else {
        // Replace "no notes" message if it exists
        container.innerHTML = '';
        container.appendChild(noteDiv);
    }
    
    // Load comments inline for non-temp notes
    if (!isTemp) {
        setTimeout(() => {
            loadReactions('note', note.id);
            loadNoteCommentsInline(note.id);
        }, 0);
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
            <div class="flex items-start gap-2 flex-1">
                ${getProfilePictureHTML(realNote, 'w-8 h-8')}
                <div class="flex-1 min-w-0">
                    <p class="text-sm font-medium text-gray-700">${realNote.first_name && realNote.last_name ? escapeHtml(`${realNote.first_name} ${realNote.last_name}`) : escapeHtml(realNote.username)}</p>
                    <p class="text-xs text-gray-500">${new Date(realNote.created_at).toLocaleString()}</p>
                </div>
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
        <div class="mt-3 pt-3 border-t border-gray-100" id="comments-section-${realNote.id}">
            <!-- Comments will be loaded here automatically -->
        </div>
    `;
    
    // Replace the temp element with the real one
    tempElement.replaceWith(noteDiv);
    
    // Load comments inline for the real note
    setTimeout(() => {
        loadReactions('note', realNote.id);
        loadNoteCommentsInline(realNote.id);
    }, 0);
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

// Load user groups for verse comments dropdown
async function loadUserGroupsForComments() {
    if (!section) return;
    
    try {
        const response = await fetch(`/api/notes/${noteId}/comments`);
        if (!response.ok) throw new Error('Failed to load comments');
        const comments = await response.json();
        
        renderNoteCommentsSection(noteId, comments, section);
    } catch (error) {
        console.error('Error loading note comments:', error);
        section.innerHTML = '<p class="text-xs text-red-500">Failed to load comments</p>';
    }
}

// Render note comments section with threading
function renderNoteCommentsSection(noteId, comments, section) {
    // Handle null or undefined comments
    const safeComments = comments || [];
    const commentCount = countNoteComments(safeComments);
    
    section.innerHTML = `
        <div class="space-y-2">
            <div style="display: flex; justify-content: space-between; align-items: center;">
                <strong style="color: #7c3aed; font-size: 0.875rem;">${commentCount} ${commentCount === 1 ? 'Comment' : 'Comments'}</strong>
                <button onclick="showAddNoteCommentForm(${noteId})" 
                        class="text-xs text-purple-600 hover:text-purple-800 font-semibold">
                    + Add Comment
                </button>
            </div>
            <div id="add-note-comment-form-${noteId}"></div>
            <div class="space-y-2">
                ${safeComments.length > 0 ? safeComments.map(comment => renderNoteComment(comment, noteId, 0)).join('') : '<p class="text-xs text-gray-500 italic">No comments yet.</p>'}
            </div>
        </div>
    `;
    
    // Load reactions for all comments after rendering
    if (safeComments.length > 0) {
        setTimeout(() => {
            loadNoteCommentReactionsRecursive(safeComments);
        }, 0);
    }
}

// Render a single note comment and its replies
function renderNoteComment(comment, noteId, depth = 0) {
    const indent = depth > 0 ? 'ml-6 border-l-2 border-purple-200 pl-3' : '';
    const canEdit = currentUserId && comment.user_id === currentUserId;
    // Store unescaped content in data attribute for editing
    const contentForEdit = comment.content.replace(/"/g, '&quot;');
    
    return `
        <div class="bg-gray-50 rounded p-2 ${indent}">
            <div class="flex items-start gap-2">
                ${getProfilePictureHTML(comment, 'w-6 h-6')}
                <div class="flex-1 min-w-0">
                    <div class="flex items-baseline justify-between">
                        <span class="font-medium text-xs text-gray-700">${comment.first_name && comment.last_name ? escapeHtml(`${comment.first_name} ${comment.last_name}`) : escapeHtml(comment.username)}</span>
                        <span class="text-xs text-gray-500">${formatDate(comment.created_at)}</span>
                    </div>
                    <p class="text-gray-800 text-xs mt-1">${escapeHtml(comment.content)}</p>
                    <div class="flex gap-3 mt-1">
                        <a class="text-xs text-purple-600 hover:text-purple-800 cursor-pointer" onclick="showReplyToNoteCommentForm(${comment.id}, ${noteId})">Reply</a>
                        ${canEdit ? `
                            <a class="text-xs text-blue-600 hover:text-blue-800 cursor-pointer" onclick="editNoteComment(${comment.id}, ${noteId}, \`${contentForEdit}\`)">Edit</a>
                            <a class="text-xs text-red-600 hover:text-red-800 cursor-pointer" onclick="deleteNoteComment(${comment.id}, ${noteId})">Delete</a>
                        ` : ''}
                    </div>
                    <div id="reply-note-comment-form-${comment.id}"></div>
                    <div id="reactions-note_comment-${comment.id}" class="flex flex-wrap gap-1 items-center mt-2">
                        <span class="text-xs text-gray-400">Loading reactions...</span>
                    </div>
                </div>
            </div>
            ${comment.replies && comment.replies.length > 0 ? '<div class="mt-2 space-y-2">' + comment.replies.map(reply => renderNoteComment(reply, noteId, depth + 1)).join('') + '</div>' : ''}
        </div>
    `;
}

// Count total note comments including replies
function countNoteComments(comments) {
    if (!comments || !Array.isArray(comments)) {
        return 0;
    }
    return comments.reduce((total, comment) => {
        return total + 1 + (comment.replies ? countNoteComments(comment.replies) : 0);
    }, 0);
}

// Load reactions recursively for note comments and their replies
function loadNoteCommentReactionsRecursive(comments) {
    if (!comments || !Array.isArray(comments)) return;
    
    comments.forEach(comment => {
        loadReactions('note_comment', comment.id);
        if (comment.replies && comment.replies.length > 0) {
            loadNoteCommentReactionsRecursive(comment.replies);
        }
    });
}

// Show form to add a comment to a note
window.showAddNoteCommentForm = function(noteId) {
    const formContainer = document.getElementById(`add-note-comment-form-${noteId}`);
    if (!formContainer) return;
    
    formContainer.innerHTML = `
        <div class="bg-white rounded p-2 border border-purple-300">
            <textarea id="new-note-comment-${noteId}" rows="2" placeholder="Add a comment..." 
                      class="w-full text-xs border border-gray-300 rounded p-2"></textarea>
            <div class="flex gap-2 mt-2">
                <button onclick="submitNoteComment(${noteId})" 
                        class="text-xs px-3 py-1 bg-purple-600 text-white rounded hover:bg-purple-700">Post</button>
                <button onclick="cancelNoteCommentForm(${noteId})" 
                        class="text-xs px-3 py-1 bg-gray-300 text-gray-700 rounded hover:bg-gray-400">Cancel</button>
            </div>
        </div>
    `;
    document.getElementById(`new-note-comment-${noteId}`).focus();
};

// Show form to reply to a comment
window.showReplyToNoteCommentForm = function(parentId, noteId) {
    const formContainer = document.getElementById(`reply-note-comment-form-${parentId}`);
    if (!formContainer) return;
    
    formContainer.innerHTML = `
        <div class="bg-white rounded p-2 border border-purple-300 mt-2">
            <textarea id="reply-note-comment-${parentId}" rows="2" placeholder="Write a reply..." 
                      class="w-full text-xs border border-gray-300 rounded p-2"></textarea>
            <div class="flex gap-2 mt-2">
                <button onclick="submitNoteCommentReply(${parentId}, ${noteId})" 
                        class="text-xs px-3 py-1 bg-purple-600 text-white rounded hover:bg-purple-700">Reply</button>
                <button onclick="cancelReplyNoteCommentForm(${parentId})" 
                        class="text-xs px-3 py-1 bg-gray-300 text-gray-700 rounded hover:bg-gray-400">Cancel</button>
            </div>
        </div>
    `;
    document.getElementById(`reply-note-comment-${parentId}`).focus();
};

// Submit a new note comment
window.submitNoteComment = async function(noteId) {
    const textarea = document.getElementById(`new-note-comment-${noteId}`);
    const content = textarea.value.trim();
    
    if (!content) return;
    
    try {
        const response = await fetch('/api/comments/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                noteId: noteId,
                content: content
            })
        });
        
        if (!response.ok) throw new Error('Failed to create comment');
        
        // Reload comments to show the new one
        await loadNoteCommentsInline(noteId);
    } catch (error) {
        console.error('Error creating comment:', error);
        showNoteMessage('Error', 'Failed to create comment');
    }
};

// Submit a reply to a note comment
window.submitNoteCommentReply = async function(parentId, noteId) {
    const textarea = document.getElementById(`reply-note-comment-${parentId}`);
    const content = textarea.value.trim();
    
    if (!content) return;
    
    try {
        const response = await fetch('/api/comments/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                noteId: noteId,
                parentId: parentId,
                content: content
            })
        });
        
        if (!response.ok) throw new Error('Failed to create reply');
        
        // Reload comments to show the new reply
        await loadNoteCommentsInline(noteId);
    } catch (error) {
        console.error('Error creating reply:', error);
        showNoteMessage('Error', 'Failed to create reply');
    }
};

// Cancel adding a comment
window.cancelNoteCommentForm = function(noteId) {
    const formContainer = document.getElementById(`add-note-comment-form-${noteId}`);
    if (formContainer) formContainer.innerHTML = '';
};

// Cancel replying to a comment
window.cancelReplyNoteCommentForm = function(parentId) {
    const formContainer = document.getElementById(`reply-note-comment-form-${parentId}`);
    if (formContainer) formContainer.innerHTML = '';
};

// Edit a note comment
window.editNoteComment = async function(commentId, noteId, currentContent) {
    showNoteEditor('Edit Comment', currentContent, async (newContent) => {
        if (!newContent || newContent === currentContent) return;
        
        try {
            const response = await fetch(`/api/comments/${commentId}/update`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ content: newContent })
            });
            
            if (!response.ok) throw new Error('Failed to update comment');
            
            await loadNoteCommentsInline(noteId);
        } catch (error) {
            console.error('Error updating comment:', error);
            showNoteMessage('Error', 'Failed to update comment');
        }
    });
};

// Delete a note comment
window.deleteNoteComment = async function(commentId, noteId) {
    showDeleteConfirm(async () => {
        try {
            const response = await fetch(`/api/comments/delete?commentId=${commentId}`, {
                method: 'DELETE'
            });
            
            if (!response.ok) throw new Error('Failed to delete comment');
            
            await loadNoteCommentsInline(noteId);
        } catch (error) {
            console.error('Error deleting comment:', error);
            showNoteMessage('Error', 'Failed to delete comment');
        }
    });
};

// Utility function for HTML escaping
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Reactions functionality
const COMMON_EMOJIS = ['👍', '❤️', '🙏', '😊', '🎉', '👏'];

// Load and display reactions for a target
async function loadReactions(targetType, targetId) {
    const container = document.getElementById(`reactions-${targetType}-${targetId}`);
    if (!container) return;

    try {
        const response = await fetch(`/api/reactions/summary?target_type=${targetType}&target_id=${targetId}`);
        if (!response.ok) throw new Error('Failed to load reactions');
        
        const reactions = await response.json() || [];
        renderReactions(targetType, targetId, reactions);
    } catch (error) {
        console.error('Error loading reactions:', error);
        container.innerHTML = '';
    }
}

// Render reactions display
function renderReactions(targetType, targetId, reactions) {
    const container = document.getElementById(`reactions-${targetType}-${targetId}`);
    if (!container) return;

    let html = '';
    
    // Display existing reactions
    if (reactions && reactions.length > 0) {
        reactions.forEach(r => {
            const buttonClass = r.has_reacted 
                ? 'bg-blue-100 border-blue-400 text-blue-800' 
                : 'bg-gray-100 border-gray-300 text-gray-700 hover:bg-gray-200';
            
            html += `
                <button onclick="toggleReaction('${targetType}', ${targetId}, '${r.emoji}')" 
                        class="inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs border transition ${buttonClass}"
                        title="${r.count} ${r.count === 1 ? 'person' : 'people'}">
                    <span>${r.emoji}</span>
                    <span class="font-medium">${r.count}</span>
                </button>
            `;
        });
    }
    
    // Add reaction button
    html += `
        <button onclick="showReactionPicker('${targetType}', ${targetId}, event)" 
                class="inline-flex items-center px-2 py-1 rounded-full text-xs border border-gray-300 bg-white hover:bg-gray-100 transition"
                title="Add reaction">
            <span>😊➕</span>
        </button>
    `;
    
    container.innerHTML = html;
}

// Toggle a reaction
async function toggleReaction(targetType, targetId, emoji) {
    try {
        const response = await fetch('/api/reactions/toggle', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                target_type: targetType,
                target_id: targetId,
                emoji: emoji
            })
        });

        if (!response.ok) throw new Error('Failed to toggle reaction');
        
        const reactions = await response.json();
        renderReactions(targetType, targetId, reactions);
    } catch (error) {
        console.error('Error toggling reaction:', error);
    }
}

// Show reaction picker
function showReactionPicker(targetType, targetId, event) {
    event.stopPropagation();
    
    // Remove any existing picker
    const existingPicker = document.querySelector('.reaction-picker');
    if (existingPicker) existingPicker.remove();
    
    // Create picker
    const picker = document.createElement('div');
    picker.className = 'reaction-picker absolute z-50 bg-white rounded-lg shadow-xl p-2 flex gap-1 border border-gray-200';
    picker.style.cssText = 'margin-top: -40px;';
    
    COMMON_EMOJIS.forEach(emoji => {
        const btn = document.createElement('button');
        btn.textContent = emoji;
        btn.className = 'text-2xl hover:scale-125 transition p-1 rounded hover:bg-gray-100';
        btn.onclick = async () => {
            await toggleReaction(targetType, targetId, emoji);
            picker.remove();
        };
        picker.appendChild(btn);
    });
    
    // Position and show picker
    const button = event.target.closest('button');
    button.parentElement.style.position = 'relative';
    button.parentElement.appendChild(picker);
    
    // Close picker on outside click
    setTimeout(() => {
        document.addEventListener('click', function closePicker(e) {
            if (!picker.contains(e.target)) {
                picker.remove();
                document.removeEventListener('click', closePicker);
            }
        });
    }, 0);
}

// ============ VERSE COMMENTS FUNCTIONALITY ============

// Load user groups for verse comments dropdown
async function loadUserGroupsForComments() {
    try {
        const response = await fetch('/api/user/groups');
        if (!response.ok) return;
        
        const groups = await response.json();
        userGroups = groups || [];
        
        // Populate the group selector
        if (verseCommentsGroupSelect) {
            verseCommentsGroupSelect.innerHTML = '<option value="">Personal Notes</option>';
            userGroups.forEach(group => {
                const option = document.createElement('option');
                option.value = group.id;
                option.textContent = group.name;
                verseCommentsGroupSelect.appendChild(option);
            });
            
            // Show verse comments controls if user is authenticated
            if (userGroups.length >= 0) {
                toggleVerseCommentsBtn.classList.remove('hidden');
                verseCommentsGroupSelect.classList.remove('hidden');
            }
        }
    } catch (error) {
        console.error('Failed to load groups for comments:', error);
    }
}

// Load comments for a specific verse
async function loadVerseComments(book, chapter, verse) {
    if (!currentUserId) return [];
    
    try {
        const params = new URLSearchParams({
            book,
            chapter,
            verse
        });
        
        if (selectedCommentGroup) {
            params.append('group_id', selectedCommentGroup);
        }
        
        const response = await fetch(`/api/verse-comments/list?${params}`);
        if (!response.ok) return [];
        
        const comments = await response.json();
        return comments || [];
    } catch (error) {
        console.error('Failed to load verse comments:', error);
        return [];
    }
}

// Render a single comment and its replies
function renderComment(comment, depth = 0) {
    const indent = depth > 0 ? 'verse-comment-reply' : '';
    return `
        <div class="verse-comment ${indent}">
            <div class="flex items-start gap-2">
                ${getProfilePictureHTML(comment, 'w-6 h-6')}
                <div class="flex-1 min-w-0">
                    <div class="verse-comment-header">
                        <strong>${escapeHtml(comment.first_name)} ${escapeHtml(comment.last_name)}</strong>
                        <span class="text-gray-500">·</span>
                        <span class="text-gray-500">${formatDate(comment.created_at)}</span>
                    </div>
                    <div class="verse-comment-content">${escapeHtml(comment.content)}</div>
                    <div class="verse-comment-actions">
                        <a class="verse-comment-action" onclick="showReplyForm(${comment.id}, '${escapeHtml(currentBook)}', ${currentChapter}, ${comment.verse})">Reply</a>
                        ${comment.user_id === currentUserId ? `
                            <a class="verse-comment-action" onclick="editVerseComment(${comment.id}, ${JSON.stringify(escapeHtml(comment.content)).replace(/"/g, '&quot;')})">Edit</a>
                            <a class="verse-comment-action text-red-600" onclick="deleteVerseComment(${comment.id})">Delete</a>
                        ` : ''}
                    </div>
                    <div id="reactions-verse_comment-${comment.id}" class="flex flex-wrap gap-1 items-center mt-2">
                        <span class="text-xs text-gray-400">Loading reactions...</span>
                    </div>
                    <div id="reply-form-${comment.id}"></div>
                </div>
            </div>
            ${comment.replies && comment.replies.length > 0 ? comment.replies.map(reply => renderComment(reply, depth + 1)).join('') : ''}
        </div>
    `;
}

// Render verse comments section
function renderVerseCommentsSection(book, chapter, verse, comments) {
    const commentCount = countTotalComments(comments);
    
    let contentHTML = '';
    
    // If no comments, show empty state
    if (commentCount === 0) {
        contentHTML = '<div id="add-comment-form-' + verse + '"></div>';
    } else {
        // If comments exist, show full section
        contentHTML = `
            <div class="verse-comments">
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem;">
                    <strong style="color: #7c3aed;">${commentCount} ${commentCount === 1 ? 'Note' : 'Notes'}</strong>
                    <button onclick="showAddCommentForm('${escapeHtml(book)}', ${chapter}, ${verse})" 
                            style="padding: 0.25rem 0.5rem; background-color: #9333ea; color: white; border-radius: 0.25rem; font-size: 0.75rem; cursor: pointer; border: none;">
                        + Add Note
                    </button>
                </div>
                <div id="add-comment-form-${verse}"></div>
                ${comments.map(comment => renderComment(comment, 0)).join('')}
            </div>
        `;
        
        // Load reactions for all verse comments after rendering
        setTimeout(() => {
            loadVerseCommentReactionsRecursive(comments);
        }, 0);
    }
    
    return `<div id="verse-comments-section-${verse}">${contentHTML}</div>`;
}

// Count total comments including replies
function countTotalComments(comments) {
    if (!comments || !Array.isArray(comments)) {
        return 0;
    }
    return comments.reduce((total, comment) => {
        return total + 1 + (comment.replies ? countTotalComments(comment.replies) : 0);
    }, 0);
}

// Load reactions recursively for verse comments and their replies
function loadVerseCommentReactionsRecursive(comments) {
    if (!comments || !Array.isArray(comments)) return;
    
    comments.forEach(comment => {
        loadReactions('verse_comment', comment.id);
        if (comment.replies && comment.replies.length > 0) {
            loadVerseCommentReactionsRecursive(comment.replies);
        }
    });
}

// Show form to add a new comment
window.showAddCommentForm = function(book, chapter, verse) {
    const formContainer = document.getElementById(`add-comment-form-${verse}`);
    if (!formContainer) return;
    
    formContainer.innerHTML = `
        <div class="verse-comment-form">
            <textarea id="new-comment-${verse}" rows="3" placeholder="Add your note..."></textarea>
            <div style="display: flex; gap: 0.5rem;">
                <button onclick="submitVerseComment('${book}', ${chapter}, ${verse})">Post</button>
                <button onclick="cancelCommentForm(${verse})" style="background-color: #6b7280;">Cancel</button>
            </div>
        </div>
    `;
    document.getElementById(`new-comment-${verse}`).focus();
};

// Show form to reply to a comment
window.showReplyForm = function(parentId, book, chapter, verse) {
    const formContainer = document.getElementById(`reply-form-${parentId}`);
    if (!formContainer) return;
    
    formContainer.innerHTML = `
        <div class="verse-comment-form" style="margin-top: 0.5rem;">
            <textarea id="reply-${parentId}" rows="2" placeholder="Write a reply..."></textarea>
            <div style="display: flex; gap: 0.5rem;">
                <button onclick="submitReply(${parentId}, '${book}', ${chapter}, ${verse})">Reply</button>
                <button onclick="cancelReplyForm(${parentId})" style="background-color: #6b7280;">Cancel</button>
            </div>
        </div>
    `;
    document.getElementById(`reply-${parentId}`).focus();
};

// Submit a new verse comment
window.submitVerseComment = async function(book, chapter, verse) {
    const textarea = document.getElementById(`new-comment-${verse}`);
    if (!textarea) return;
    
    const content = textarea.value.trim();
    if (!content) return;
    
    try {
        const response = await fetch('/api/verse-comments/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                book,
                chapter,
                verse,
                content,
                group_id: selectedCommentGroup || null
            })
        });
        
        if (response.ok) {
            await reloadVerseComments(book, chapter, verse); // Reload just this verse's comments
        } else {
            showNoteMessage('Error', 'Failed to add note');
        }
    } catch (error) {
        console.error('Failed to submit comment:', error);
        showNoteMessage('Error', 'Failed to add note');
    }
};

// Submit a reply to a comment
window.submitReply = async function(parentId, book, chapter, verse) {
    const textarea = document.getElementById(`reply-${parentId}`);
    if (!textarea) return;
    
    const content = textarea.value.trim();
    if (!content) return;
    
    try {
        const response = await fetch('/api/verse-comments/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                book,
                chapter,
                verse,
                content,
                parent_id: parentId,
                group_id: selectedCommentGroup || null
            })
        });
        
        if (response.ok) {
            await reloadVerseComments(book, chapter, verse); // Reload just this verse's comments
        } else {
            showNoteMessage('Error', 'Failed to add reply');
        }
    } catch (error) {
        console.error('Failed to submit reply:', error);
        showNoteMessage('Error', 'Failed to add reply');
    }
};

// Cancel comment form
window.cancelCommentForm = function(verse) {
    const formContainer = document.getElementById(`add-comment-form-${verse}`);
    if (formContainer) formContainer.innerHTML = '';
};

// Cancel reply form
window.cancelReplyForm = function(parentId) {
    const formContainer = document.getElementById(`reply-form-${parentId}`);
    if (formContainer) formContainer.innerHTML = '';
};

// Delete a verse comment
window.deleteVerseComment = async function(commentId) {
    showDeleteConfirm(async () => {
        try {
            const response = await fetch(`/api/verse-comments/${commentId}/delete`, {
                method: 'DELETE'
            });
            
            if (response.ok) {
                // Reload just the verse comments for the current view
                await reloadAllVisibleVerseComments();
            } else {
                showNoteMessage('Error', 'Failed to delete note');
            }
        } catch (error) {
            console.error('Failed to delete comment:', error);
            showNoteMessage('Error', 'Failed to delete note');
        }
    });
};

// Edit a verse comment
window.editVerseComment = function(commentId, currentContent) {
    showNoteEditor('Edit Note', currentContent, async (newContent) => {
        try {
            const response = await fetch(`/api/verse-comments/${commentId}/update`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ content: newContent })
            });
            
            if (response.ok) {
                // Reload just the verse comments for the current view
                await reloadAllVisibleVerseComments();
            } else {
                showNoteMessage('Error', 'Failed to update note');
            }
        } catch (error) {
            console.error('Failed to edit comment:', error);
            showNoteMessage('Error', 'Failed to update note');
        }
    });
};

// Reload verse comments for a specific verse
async function reloadVerseComments(book, chapter, verse) {
    const section = document.getElementById(`verse-comments-section-${verse}`);
    if (!section) return;
    
    try {
        const comments = await loadVerseComments(book, chapter, verse);
        const commentCount = countTotalComments(comments);
        
        let contentHTML = '';
        
        // If no comments, show empty state
        if (commentCount === 0) {
            contentHTML = '<div id="add-comment-form-' + verse + '"></div>';
        } else {
            // If comments exist, show full section
            contentHTML = `
                <div class="verse-comments">
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem;">
                        <strong style="color: #7c3aed;">${commentCount} ${commentCount === 1 ? 'Note' : 'Notes'}</strong>
                        <button onclick="showAddCommentForm('${escapeHtml(book)}', ${chapter}, ${verse})" 
                                style="padding: 0.25rem 0.5rem; background-color: #9333ea; color: white; border-radius: 0.25rem; font-size: 0.75rem; cursor: pointer; border: none;">
                            + Add Note
                        </button>
                    </div>
                    <div id="add-comment-form-${verse}"></div>
                    ${comments.map(comment => renderComment(comment, 0)).join('')}
                </div>
            `;
        }
        
        // Update the section's innerHTML
        section.innerHTML = contentHTML;
        
        // Load reactions after DOM is updated
        if (comments.length > 0) {
            setTimeout(() => {
                loadVerseCommentReactionsRecursive(comments);
            }, 0);
        }
    } catch (error) {
        console.error('Failed to reload verse comments:', error);
    }
}

// Reload all visible verse comments in the current chapter
async function reloadAllVisibleVerseComments() {
    if (!showVerseComments || !currentBook || !currentChapter) return;
    
    const verseDivs = document.querySelectorAll('[data-verse]');
    for (const verseDiv of verseDivs) {
        const verse = parseInt(verseDiv.getAttribute('data-verse'));
        if (verse) {
            await reloadVerseComments(currentBook, currentChapter, verse);
        }
    }
}

// Format date for comments
function formatDate(dateString) {
    const date = new Date(dateString);
    const now = new Date();
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    
    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    if (diffDays < 7) return `${diffDays}d ago`;
    return date.toLocaleDateString();
}

// WebSocket connection for real-time updates
let ws = null;
let wsReconnectTimeout = null;

function connectWebSocket() {
    // Only connect if user is authenticated
    if (!currentUserId) return;
    
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;
    
    try {
        ws = new WebSocket(wsUrl);
        
        ws.onopen = function() {
            console.log('WebSocket connected');
            
            // Clear any reconnection timeout
            if (wsReconnectTimeout) {
                clearTimeout(wsReconnectTimeout);
                wsReconnectTimeout = null;
            }
        };
        
        ws.onmessage = function(event) {
            try {
                const message = JSON.parse(event.data);
                handleWebSocketMessage(message);
            } catch (error) {
                console.error('Failed to parse WebSocket message:', error);
            }
        };
        
        ws.onerror = function(error) {
            console.error('WebSocket error:', error);
        };
        
        ws.onclose = function() {
            console.log('WebSocket disconnected');
            ws = null;
            
            // Reconnect after 3 seconds if user is still authenticated
            if (currentUserId && !wsReconnectTimeout) {
                wsReconnectTimeout = setTimeout(connectWebSocket, 3000);
            }
        };
    } catch (error) {
        console.error('Failed to create WebSocket:', error);
    }
}

function handleWebSocketMessage(message) {
    const { type, action, book, chapter, verse, note_id, data } = message;
    
    // Handle verse comments
    if (type === 'verse_comment') {
        if (showVerseComments && currentBook === book && currentChapter === chapter && verse) {
            // Reload just the affected verse's comments
            reloadVerseComments(book, chapter, verse);
        }
    }
    
    // Handle note comments
    if (type === 'note_comment' && note_id) {
        const section = document.getElementById(`comments-section-${note_id}`);
        if (section) {
            // Reload note comments if visible
            loadNoteCommentsInline(note_id);
        }
    }
    
    // Handle reactions
    if (type === 'reaction' && data) {
        const { target_type, target_id } = data;
        if (target_type && target_id) {
            // Reload reactions for this target
            loadReactions(target_type, target_id);
        }
    }
}

// Start the application
init();

