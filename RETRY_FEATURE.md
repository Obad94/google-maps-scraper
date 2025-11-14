# Job Retry Feature

The Google Maps Scraper now supports retrying failed jobs across all interfaces: Web UI, REST API, and CLI.

## Overview

When a job fails (due to timeout, errors, or being stuck), you can now retry it without having to recreate the job from scratch. The retry feature:

- Only works on jobs with status `failed`
- Deletes any partial CSV data from the failed run
- Changes the job status to `pending` so it will be picked up by the worker
- Preserves all original job settings (keywords, language, depth, etc.)

---

## How to Use

### 1. Web UI (Easiest Method)

1. **Navigate to** `http://localhost:8080` in your browser
2. **Find the failed job** in the jobs table (marked with red "failed" status)
3. **Click the orange "Retry" button** next to the failed job
4. The job will be automatically re-queued and picked up by the worker

**Visual Example:**
```
Job ID                                    Job Name    Status    Actions
c2d94df1-cb3e-481a-a6ba-a87b164c434e     t14         failed    [Retry] [Delete]
                                                               ↑ Click this!
```

---

### 2. REST API

**Endpoint:** `POST /api/v1/jobs/{id}/retry`

#### cURL Example:
```bash
curl -X POST "http://localhost:8080/api/v1/jobs/{JOB_ID}/retry"
```

#### Full Example with Job ID:
```bash
curl -X POST "http://localhost:8080/api/v1/jobs/c2d94df1-cb3e-481a-a6ba-a87b164c434e/retry"
```

#### Response (Success - 200 OK):
```json
{
  "id": "c2d94df1-cb3e-481a-a6ba-a87b164c434e",
  "name": "t14",
  "date": "2025-11-14T13:47:57Z",
  "status": "pending",
  "data": {
    "keywords": ["restaurants in Hanoi"],
    "lang": "en",
    "depth": 1,
    "zoom": 15,
    ...
  }
}
```

#### Response (Error - 400 Bad Request):
```json
{
  "code": 400,
  "message": "only failed jobs can be retried, current status: ok"
}
```

#### Using with JavaScript/Fetch:
```javascript
const jobId = 'c2d94df1-cb3e-481a-a6ba-a87b164c434e';

fetch(`http://localhost:8080/api/v1/jobs/${jobId}/retry`, {
  method: 'POST'
})
.then(response => response.json())
.then(job => {
  console.log('Job retried successfully:', job);
  console.log('New status:', job.status); // Should be 'pending'
})
.catch(error => console.error('Error:', error));
```

#### Using with Python:
```python
import requests

job_id = 'c2d94df1-cb3e-481a-a6ba-a87b164c434e'
url = f'http://localhost:8080/api/v1/jobs/{job_id}/retry'

response = requests.post(url)

if response.status_code == 200:
    job = response.json()
    print(f"Job retried successfully: {job['name']}")
    print(f"New status: {job['status']}")
else:
    error = response.json()
    print(f"Error: {error['message']}")
```

---

### 3. CLI / Command Line

The CLI doesn't have a direct retry command, but you can use `curl` to interact with the API:

#### Retry a Single Job:
```bash
curl -X POST "http://localhost:8080/api/v1/jobs/{JOB_ID}/retry"
```

#### Retry All Failed Jobs (Bash Script):
```bash
#!/bin/bash

# Get all jobs
JOBS=$(curl -s "http://localhost:8080/api/v1/jobs")

# Extract failed job IDs and retry them
echo "$JOBS" | jq -r '.[] | select(.status == "failed") | .id' | while read JOB_ID; do
    echo "Retrying job: $JOB_ID"
    curl -X POST "http://localhost:8080/api/v1/jobs/$JOB_ID/retry"
    echo ""
done

echo "All failed jobs have been retried!"
```

#### Check Job Status:
```bash
# Get specific job details
curl "http://localhost:8080/api/v1/jobs/{JOB_ID}"

# Get all jobs and their statuses
curl "http://localhost:8080/api/v1/jobs" | jq '.[] | {id: .id, name: .name, status: .status}'
```

---

## API Documentation

Full API documentation is available at:
- **Swagger UI** (Interactive): http://localhost:8080/api/swagger
- **Redoc**: http://localhost:8080/api/docs

Search for the `/api/v1/jobs/{id}/retry` endpoint in the documentation.

---

## Workflow Example

### Complete Retry Workflow:

1. **Create a job:**
   ```bash
   curl -X POST "http://localhost:8080/api/v1/jobs" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "Coffee shops in Paris",
       "keywords": ["coffee shops in Paris"],
       "lang": "en",
       "depth": 1
     }'
   ```

2. **Job fails** (timeout, error, etc.) → Status becomes `failed`

3. **Retry the job:**
   ```bash
   curl -X POST "http://localhost:8080/api/v1/jobs/{JOB_ID}/retry"
   ```

4. **Job status changes to `pending`** → Worker picks it up automatically

5. **Job runs again** → Status becomes `working` → Eventually `ok` or `failed`

---

## Error Handling

### Common Errors:

#### 1. Job Not in Failed Status:
```json
{
  "code": 400,
  "message": "only failed jobs can be retried, current status: ok"
}
```
**Solution:** Only failed jobs can be retried. Check the job status first.

#### 2. Job Not Found:
```json
{
  "code": 404,
  "message": "job not found"
}
```
**Solution:** Verify the job ID is correct.

#### 3. Invalid Job ID:
```json
{
  "code": 422,
  "message": "Invalid ID"
}
```
**Solution:** Use a valid UUID format for the job ID.

---

## Notes

- **Automatic Cleanup:** When you retry a job, any partial CSV data from the failed run is automatically deleted
- **Original Settings:** The retry uses the exact same settings (keywords, language, depth, etc.) as the original job
- **No Duplication:** Retry doesn't create a new job; it re-uses the existing job
- **Worker Required:** Make sure the web server is running for the retry to be processed
- **Status Flow:** `failed` → (retry) → `pending` → `working` → `ok` or `failed`

---

## Comparison: Retry vs Delete + Create New Job

| Feature | Retry | Delete + Create New |
|---------|-------|---------------------|
| Keeps job history | ✅ Yes | ❌ No |
| Preserves job ID | ✅ Yes | ❌ New ID generated |
| One-click operation | ✅ Yes | ❌ Multiple steps |
| Cleans up partial data | ✅ Automatic | ⚠️ Manual |
| API calls needed | 1 | 2 (DELETE + POST) |

---

## Docker Usage

When using Docker, the retry feature works the same way:

```bash
# Start the scraper
docker run -v ${PWD}\gmapsdata:/gmapsdata -p 8080:8080 google-maps-scraper -data-folder /gmapsdata

# Then use any of the methods above to retry jobs
# Example:
curl -X POST "http://localhost:8080/api/v1/jobs/{JOB_ID}/retry"
```

---

## Troubleshooting

### Retry button not appearing in UI:
- Make sure you've rebuilt the Docker image or restarted the server
- Check that the job status is actually `failed` (not `ok` or `working`)

### Retry not picking up the job:
- Verify the worker is running (check logs for "job worker started")
- Check that no other job is currently running (concurrency limit)
- Look for errors in the server logs

### CLI retry not working:
- Ensure the server is running at http://localhost:8080
- Verify the job ID is correct using: `curl "http://localhost:8080/api/v1/jobs"`
- Check if `curl` is installed on your system

---

## Additional Resources

- [Main README](README.md)
- [API Documentation](http://localhost:8080/api/swagger)
- [GitHub Issues](https://github.com/gosom/google-maps-scraper/issues)
