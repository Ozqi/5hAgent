---
name: code-review
description: Use when user asks to "review code", "check quality", "audit security", "improve performance", or mentions code review. Provides comprehensive code review checklist.
---

# Code Review

Comprehensive code review focusing on quality, security, and performance.

## When to Use

This skill activates when:
- User requests code review or quality check
- Security audit is needed
- Performance optimization is discussed
- Code standards compliance is required

## Review Checklist

### 1. Code Quality

**Readability:**
- Clear variable and function names
- Appropriate comments (only for non-obvious WHY, not WHAT)
- Consistent formatting and style
- Logical code organization

**Maintainability:**
- DRY principle (Don't Repeat Yourself)
- Single Responsibility Principle
- Appropriate abstraction levels
- No premature optimization

**Error Handling:**
- Proper error catching and handling
- Meaningful error messages
- Graceful degradation
- Resource cleanup (files, connections, locks)

### 2. Security

**Input Validation:**
- All user input is validated and sanitized
- SQL injection prevention (parameterized queries)
- XSS prevention (output encoding)
- CSRF protection where applicable

**Authentication & Authorization:**
- Proper authentication checks
- Authorization before sensitive operations
- Secure session management
- Password handling (hashing, not plaintext)

**Data Protection:**
- Sensitive data encryption
- No secrets in code (use environment variables)
- Secure communication (HTTPS, TLS)
- Proper access controls

### 3. Performance

**Efficiency:**
- No unnecessary computations
- Efficient algorithms and data structures
- Avoid N+1 query problems
- Proper indexing for database queries

**Resource Management:**
- No memory leaks
- Connection pooling where appropriate
- Lazy loading for expensive operations
- Caching for frequently accessed data

**Concurrency:**
- Thread-safe code where needed
- Proper locking mechanisms
- Avoid race conditions
- Parallel execution where beneficial

### 4. Testing

**Test Coverage:**
- Unit tests for core logic
- Integration tests for workflows
- Edge cases covered
- Error conditions tested

**Test Quality:**
- Tests are independent
- Clear test names
- Minimal test setup
- Fast execution

## Review Process

1. **Understand the context**: What problem does this code solve?
2. **Check functionality**: Does it work as intended?
3. **Review systematically**: Go through each checklist item
4. **Prioritize findings**: Critical security issues first
5. **Provide actionable feedback**: Specific suggestions, not just problems
6. **Acknowledge good practices**: Note what's done well

## Common Issues to Flag

- Hardcoded credentials or secrets
- Missing input validation
- Inefficient database queries
- Memory leaks or resource leaks
- Race conditions
- Missing error handling
- Code duplication
- Overly complex logic
- Missing tests for critical paths
