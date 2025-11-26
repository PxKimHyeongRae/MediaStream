# 10개의 FFmpeg RTSP 스트림 생성 스크립트

Write-Host "Starting 10 FFmpeg RTSP streams..." -ForegroundColor Green

for ($i = 1; $i -le 10; $i++) {
    $streamName = "stream$i"
    Write-Host "Starting $streamName..." -ForegroundColor Cyan

    Start-Process -FilePath "ffmpeg" `
        -ArgumentList "-re", "-stream_loop", "-1", "-i", "bunny.mp4", `
                     "-c:v", "copy", "-c:a", "copy", `
                     "-f", "rtsp", "rtsp://localhost:8554/$streamName" `
        -WindowStyle Hidden

    Start-Sleep -Milliseconds 500
}

Write-Host "All 10 streams started!" -ForegroundColor Green
Write-Host "Waiting 3 seconds for streams to stabilize..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

Write-Host "`nStreams running:" -ForegroundColor Green
Get-Process ffmpeg | Select-Object Id, ProcessName, StartTime
