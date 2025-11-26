package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	numStreams := 3
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &numStreams)
	}

	fmt.Printf("Starting %d FFmpeg RTSP streams...\n", numStreams)

	for i := 1; i <= numStreams; i++ {
		streamName := fmt.Sprintf("stream%d", i)
		rtspURL := fmt.Sprintf("rtsp://localhost:8554/%s", streamName)

		cmd := exec.Command("ffmpeg",
			"-re", "-stream_loop", "-1",
			"-i", "bunny.mp4",
			"-c:v", "copy", "-c:a", "copy",
			"-f", "rtsp", rtspURL)

		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Start(); err != nil {
			fmt.Printf("Failed to start %s: %v\n", streamName, err)
			continue
		}

		fmt.Printf("✅ Started %s (PID: %d)\n", streamName, cmd.Process.Pid)
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Printf("\n✅ All %d streams started!\n", numStreams)
	fmt.Println("Press Ctrl+C to stop all streams")

	// Keep running
	select {}
}
