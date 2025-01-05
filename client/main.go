package main

import (
	"BS-client/network"
	"flag"
	"fmt"
	"os"
)

func main() {
	// Command line flags for server address and file path

	//Sub commands
	uploadCmd := flag.NewFlagSet("upload", flag.ExitOnError)
	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
	deleteCmd := flag.NewFlagSet("delete", flag.ExitOnError)

	// Subcommand flags (each set of flags is scoped to its subcommand)
	// Upload
	uploadFile := uploadCmd.String("file", "", "Path of the file to send")
	uploadDest := uploadCmd.String("dest", "", "Destination to send file to (addr:port)")

	// Download
	downloadURL := downloadCmd.String("url", "", "URL of the file to download")
	downloadDest := downloadCmd.String("dest", "", "Destination to download file from (addr:port)")

	// Delete
	deleteID := deleteCmd.String("ID", "", "ID of the file to delete")
	deleteDest := deleteCmd.String("dest", "", "Destination to delete file from (addr:port)")

	// aquire at least one subcommand
	if len(os.Args) < 2 {
		fmt.Println("subcommand is required")
		os.Exit(1)
	}

	// Switch on the subcommand
	switch os.Args[1] {

	case "upload":
		// Parse the flags for upload
		uploadCmd.Parse(os.Args[2:])

		// Validate required flags
		if *uploadFile == "" || *uploadDest == "" {
			fmt.Println("Usage: send -file <path> -dest <addr:port>")
			os.Exit(1)
		}

		fmt.Printf("send file: %s to destination: %s\n", *uploadFile, *uploadDest)
		// LOGIC FOR UPLOAD COMMAND
		go network.UploadLogic(uploadFile, uploadDest)

	case "download":
		// Parse the flags for download
		downloadCmd.Parse(os.Args[2:])

		// Validate required flags
		if *downloadURL == "" || *downloadDest == "" {
			fmt.Println("Usage: download -url <url> -dest <addr:port>")
			os.Exit(1)
		}

		fmt.Printf("downloading file: %s from destination: %s\n", *downloadURL, *downloadDest)

	case "delete":
		// Parse the flags for delete
		deleteCmd.Parse(os.Args[2:])
		// Validate required flags
		if *deleteID == "" || *deleteDest == "" {
			fmt.Println("Usage: delete -ID <ID> -dest <addr:port>")
			os.Exit(1)
		}

		fmt.Printf("deleting file with ID: %s from destination: %s\n", *deleteID, *deleteDest)

	default:
		// if the user didnt enter a valid subcommand, show usage and exit
		fmt.Println("Expected 'upload', 'download', or 'delete' commands")
		os.Exit(1)
	}

}
