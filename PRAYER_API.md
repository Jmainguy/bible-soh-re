# Prayer Requests API Documentation

## Overview
Prayer requests allow group members to share prayer needs, track answered prayers, and support each other through comments.

## Database Schema

### prayer_requests Table
- `id` - Unique identifier
- `group_id` - Reference to study group
- `user_id` - User who created the request
- `title` - Short title/summary
- `content` - Detailed prayer request
- `status` - "active", "answered", or "archived"
- `created_at` - Timestamp of creation
- `updated_at` - Timestamp of last update

### prayer_comments Table
- `id` - Unique identifier
- `prayer_id` - Reference to prayer request
- `user_id` - User who wrote the comment
- `content` - Comment/prayer content
- `created_at` - Timestamp of creation
- `updated_at` - Timestamp of last update

## API Endpoints

### Create Prayer Request
**POST** `/api/prayers/create`

Creates a new prayer request for a group.

**Request Body:**
```json
{
  "group_id": 1,
  "title": "Health concerns",
  "content": "Please pray for my grandmother who is in the hospital..."
}
```

**Response:** `200 OK`
```json
{
  "id": 1,
  "group_id": 1,
  "user_id": 5,
  "title": "Health concerns",
  "content": "Please pray for my grandmother...",
  "status": "active",
  "created_at": "2026-02-06T10:30:00Z",
  "updated_at": "2026-02-06T10:30:00Z",
  "username": "john_smith"
}
```

**Authorization:** Must be a group member

---

### Get Group Prayer Requests
**GET** `/api/prayers/group?group_id={groupId}[&status={status}]`

Retrieves all prayer requests for a group, optionally filtered by status.

**Query Parameters:**
- `group_id` (required) - Group ID
- `status` (optional) - Filter by status: "active", "answered", or "archived"

**Response:** `200 OK`
```json
[
  {
    "id": 1,
    "group_id": 1,
    "user_id": 5,
    "title": "Health concerns",
    "content": "Please pray...",
    "status": "active",
    "created_at": "2026-02-06T10:30:00Z",
    "updated_at": "2026-02-06T10:30:00Z",
    "username": "john_smith"
  }
]
```

**Authorization:** Must be a group member

---

### Get Single Prayer Request
**GET** `/api/prayers/{id}`

Retrieves a specific prayer request by ID.

**Response:** `200 OK`
```json
{
  "id": 1,
  "group_id": 1,
  "user_id": 5,
  "title": "Health concerns",
  "content": "Please pray...",
  "status": "active",
  "created_at": "2026-02-06T10:30:00Z",
  "updated_at": "2026-02-06T10:30:00Z",
  "username": "john_smith"
}
```

**Authorization:** Must be a group member

---

### Update Prayer Request
**PUT** `/api/prayers/{id}/update`

Updates the title and content of a prayer request.

**Request Body:**
```json
{
  "title": "Update on grandmother",
  "content": "She's feeling better! Thank you for prayers..."
}
```

**Response:** `200 OK`
```json
{
  "success": true
}
```

**Authorization:** Must be the original creator

---

### Update Prayer Request Status
**PATCH** `/api/prayers/{id}/status`

Updates the status of a prayer request.

**Request Body:**
```json
{
  "status": "answered"
}
```

**Valid statuses:**
- `"active"` - Still in need of prayer
- `"answered"` - Prayer has been answered
- `"archived"` - No longer active but not necessarily answered

**Response:** `200 OK`
```json
{
  "success": true
}
```

**Authorization:** Must be the original creator

---

### Delete Prayer Request
**DELETE** `/api/prayers/{id}/delete`

Deletes a prayer request and all associated comments.

**Response:** `200 OK`
```json
{
  "success": true
}
```

**Authorization:** Must be the original creator

---

### Create Prayer Comment
**POST** `/api/prayers/{id}/comments`

Adds a comment/prayer to a prayer request.

**Request Body:**
```json
{
  "content": "Praying for your grandmother! May God bring healing..."
}
```

**Response:** `200 OK`
```json
{
  "id": 10,
  "prayer_id": 1,
  "user_id": 7,
  "content": "Praying for your grandmother!...",
  "created_at": "2026-02-06T11:00:00Z",
  "updated_at": "2026-02-06T11:00:00Z",
  "username": "sarah_jones"
}
```

**Authorization:** Must be a group member

---

### Get Prayer Comments
**GET** `/api/prayers/{id}/comments`

Retrieves all comments for a prayer request.

**Response:** `200 OK`
```json
[
  {
    "id": 10,
    "prayer_id": 1,
    "user_id": 7,
    "content": "Praying for your grandmother!...",
    "created_at": "2026-02-06T11:00:00Z",
    "updated_at": "2026-02-06T11:00:00Z",
    "username": "sarah_jones"
  }
]
```

**Authorization:** Must be a group member

---

### Delete Prayer Comment
**DELETE** `/api/prayer-comments/delete?commentId={commentId}`

Deletes a comment on a prayer request.

**Response:** `200 OK`
```json
{
  "success": true
}
```

**Authorization:** Must be the comment author

---

## Error Responses

All endpoints may return the following error responses:

**401 Unauthorized**
```
Unauthorized
```
User is not logged in.

**403 Forbidden**
```
Not a member of this group
```
or
```
Not authorized
```
User doesn't have permission for this action.

**404 Not Found**
```
Prayer request not found
```

**400 Bad Request**
```
Title and content are required
```
or
```
Invalid status filter
```

**500 Internal Server Error**
```
Failed to create prayer request
```

---

## Usage Examples

### JavaScript/Fetch Example

```javascript
// Create a prayer request
async function createPrayerRequest(groupId, title, content) {
  const response = await fetch('/api/prayers/create', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      group_id: groupId,
      title: title,
      content: content
    })
  });
  return await response.json();
}

// Get all active prayer requests for a group
async function getActivePrayers(groupId) {
  const response = await fetch(`/api/prayers/group?group_id=${groupId}&status=active`);
  return await response.json();
}

// Mark a prayer as answered
async function markAnswered(prayerId) {
  const response = await fetch(`/api/prayers/${prayerId}/status`, {
    method: 'PATCH',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      status: 'answered'
    })
  });
  return await response.json();
}

// Add a comment/prayer
async function addPrayer(prayerId, content) {
  const response = await fetch(`/api/prayers/${prayerId}/comments`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      content: content
    })
  });
  return await response.json();
}
```

### cURL Examples

```bash
# Create prayer request
curl -X POST http://localhost:8080/api/prayers/create \
  -H "Content-Type: application/json" \
  -d '{
    "group_id": 1,
    "title": "Travel safety",
    "content": "Traveling to visit family this weekend"
  }'

# Get all prayers for a group
curl http://localhost:8080/api/prayers/group?group_id=1

# Get only answered prayers
curl http://localhost:8080/api/prayers/group?group_id=1&status=answered

# Update prayer status
curl -X PATCH http://localhost:8080/api/prayers/1/status \
  -H "Content-Type: application/json" \
  -d '{"status": "answered"}'

# Add a comment
curl -X POST http://localhost:8080/api/prayers/1/comments \
  -H "Content-Type: application/json" \
  -d '{"content": "Praying for safe travels!"}'
```

---

## Frontend Integration Tips

### Displaying Prayer Requests

1. **Filter by Status**: Allow users to view active, answered, or all prayers
2. **Show Username**: Display who posted each request
3. **Timestamp**: Use relative time ("2 hours ago") for better UX
4. **Comment Count**: Show number of prayers/comments on each request

### Status Badges

Use visual indicators for prayer status:
- 🙏 Active - Blue badge
- ✅ Answered - Green badge  
- 📁 Archived - Gray badge

### Real-time Updates

Consider implementing:
- Automatic refresh of prayer list every 30-60 seconds
- New prayer notification
- Update notification when status changes

### Mobile Considerations

- Swipe actions (swipe to mark as answered)
- Quick prayer button (tap to add "Praying 🙏")
- Push notifications for new requests

---

## Database Migration

If you have an existing database, you'll need to run migrations to add the new tables. The application will automatically create the tables on startup if using the NewDatabase() function.

To manually create the tables:

```sql
CREATE TABLE IF NOT EXISTS prayer_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (group_id) REFERENCES study_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_prayer_requests_group ON prayer_requests(group_id);
CREATE INDEX IF NOT EXISTS idx_prayer_requests_status ON prayer_requests(group_id, status);

CREATE TABLE IF NOT EXISTS prayer_comments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    prayer_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (prayer_id) REFERENCES prayer_requests(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_prayer_comments_prayer ON prayer_comments(prayer_id);
```
