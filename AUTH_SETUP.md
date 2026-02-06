# Authentication Setup Guide

This application now supports user authentication with the following methods:
- Username and password (local authentication)
- Google OAuth
- GitHub OAuth

## Quick Start

### 1. Local Authentication (Username/Password)

Local authentication works out of the box without any configuration. Users can:
- Register at `/register`
- Login at `/login`
- Use the application as authenticated users

### 2. OAuth Setup (Optional)

To enable Google and GitHub OAuth, you need to obtain OAuth credentials and update your `config.yaml` file.

#### Google OAuth Setup

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Enable the Google+ API
4. Go to "Credentials" → "Create Credentials" → "OAuth 2.0 Client ID"
5. Configure the OAuth consent screen
6. For Application type, select "Web application"
7. Add authorized redirect URIs:
   - `http://localhost:8080/auth/google/callback` (for local development)
   - `https://yourdomain.com/auth/google/callback` (for production)
8. Copy the Client ID and Client Secret
9. Update `config.yaml`:
   ```yaml
   auth:
     baseURL: "http://localhost:8080"  # Change for production
     googleClientID: "YOUR_GOOGLE_CLIENT_ID"
     googleClientSecret: "YOUR_GOOGLE_CLIENT_SECRET"
   ```

#### GitHub OAuth Setup

1. Go to [GitHub Developer Settings](https://github.com/settings/developers)
2. Click "New OAuth App"
3. Fill in the application details:
   - Application name: Bible Reader (or your choice)
   - Homepage URL: `http://localhost:8080` (change for production)
   - Authorization callback URL: `http://localhost:8080/auth/github/callback`
4. Copy the Client ID and generate a Client Secret
5. Update `config.yaml`:
   ```yaml
   auth:
     baseURL: "http://localhost:8080"  # Change for production
     githubClientID: "YOUR_GITHUB_CLIENT_ID"
     githubClientSecret: "YOUR_GITHUB_CLIENT_SECRET"
   ```

### 3. Session Secret

For production deployments, it's important to set a secure session secret:

```yaml
auth:
  sessionSecret: "GENERATE-A-LONG-RANDOM-STRING-HERE"
```

You can generate a random secret using:
```bash
openssl rand -base64 32
```

## Configuration File Example

Here's a complete example of the auth section in `config.yaml`:

```yaml
auth:
  # Base URL of your application (used for OAuth callbacks)
  baseURL: "http://localhost:8080"
  
  # Session secret key (generate a random string in production)
  sessionSecret: "your-random-secret-key-here"
  
  # Google OAuth credentials
  googleClientID: "your-google-client-id"
  googleClientSecret: "your-google-client-secret"
  
  # GitHub OAuth credentials
  githubClientID: "your-github-client-id"
  githubClientSecret: "your-github-client-secret"
```

## Features

### User Authentication
- **Local Registration**: Users can create accounts with email, username, and password
- **Local Login**: Users can login with username/email and password
- **OAuth Login**: Users can sign in with their Google or GitHub accounts
- **Persistent Sessions**: Sessions last for 30 days
- **Secure Password Storage**: Passwords are hashed using bcrypt

### Database
- User data is stored in a SQLite database (`bible.db`)
- The database is automatically created on first run
- Session management with automatic cleanup of expired sessions

### Security Features
- Passwords hashed with bcrypt
- HttpOnly cookies for session management
- CSRF protection for OAuth state tokens
- Session expiration after 30 days of inactivity

### Reading Position Memory
- **Automatic Saving**: Your reading position (book, chapter, translation) is automatically saved as you read
- **Cross-Device Sync**: For authenticated users, reading position syncs across devices via the database
- **Local Storage**: Non-authenticated users still get position memory via browser localStorage
- **Smart Resume**: When you return to the site, you'll automatically continue where you left off
- **Priority System**: URL parameters > saved position > default (Genesis 1)

## API Endpoints

### Authentication Endpoints
- `GET /login` - Login page
- `GET /register` - Registration page
- `POST /auth/register` - Handle registration
- `POST /auth/login` - Handle login
- `GET /auth/logout` - Logout user
- `GET /auth/google` - Initiate Google OAuth
- `GET /auth/google/callback` - Google OAuth callback
- `GET /auth/github` - Initiate GitHub OAuth
- `GET /auth/github/callback` - GitHub OAuth callback
- `GET /api/user` - Get current user info (JSON)
- `GET /api/reading-position` - Get user's saved reading position (JSON)
- `POST /api/save-reading-position` - Save user's reading position (JSON)

## Usage

### Running the Application

```bash
# Build the application
go build

# Run the application
./bible-soh-re
```

The server will start on `http://localhost:8080`

### User Flow

1. **First-time visitors** can either:
   - Continue without signing in (use the app anonymously)
   - Click "Sign In" to access the login page
   - Click "Sign up" from the login page to create an account

2. **Registration** (local):
   - Enter email, username, and password
   - Password must be at least 8 characters
   - Username must be 3-20 characters (letters, numbers, underscores, hyphens)

3. **Login** (local):
   - Enter email or username
   - Enter password
   - Click "Sign In"

4. **OAuth Login**:
   - Click "Continue with Google" or "Continue with GitHub"
   - Authorize the application
   - You'll be redirected back and automatically logged in

5. **Authenticated users** see:
   - Their username in the header
   - A dropdown menu to sign out

## Production Deployment

For production deployments, remember to:

1. **Update baseURL** in `config.yaml` to your domain
2. **Set secure session secret** using a random string
3. **Update OAuth redirect URIs** in Google/GitHub settings
4. **Enable HTTPS** and set `Secure: true` for cookies in [auth.go](auth.go#L99)
5. **Backup the database** regularly (`bible.db`)
6. **Set proper file permissions** for `config.yaml` and `bible.db`

## Troubleshooting

### OAuth Issues
- Ensure your redirect URIs match exactly (including http/https)
- Check that your OAuth credentials are correct
- Verify the baseURL in config.yaml matches your deployment URL

### Database Issues
- Check file permissions for `bible.db`
- Ensure the application has write access to the current directory

### Session Issues
- Clear browser cookies if having login issues
- Check that sessions aren't expired (30-day limit)

## Future Enhancements

Potential improvements for the authentication system:
- Email verification
- Password reset functionality
- User profile management
- Remember me functionality
- Two-factor authentication
- Social login with more providers (Microsoft, Apple, etc.)
- Admin panel for user management
