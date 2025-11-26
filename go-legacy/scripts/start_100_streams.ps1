# Start 100 FFmpeg RTSP streams

Write-Host "Starting 100 FFmpeg RTSP streams..." -ForegroundColor Green

for ($i = 1; $i -le 100; $i++) {
    $streamName = "stream$i"

    Start-Process -FilePath "ffmpeg" `
        -ArgumentList "-re", "-stream_loop", "-1", "-i", "bunny.mp4", `
                     "-c:v", "copy", "-c:a", "copy", `
                     "-f", "rtsp", "rtsp://localhost:8554/$streamName" `
        -WindowStyle Hidden `
        -RedirectStandardOutput NUL `
        -RedirectStandardError NUL

    Start-Sleep -Milliseconds 200

    if ($i % 10 -eq 0) {
        Write-Host "Started $i streams..." -ForegroundColor Cyan
    }
}

Write-Host "All 100 streams started!" -ForegroundColor Green
