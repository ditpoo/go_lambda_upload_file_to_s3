package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/reecerussell/aws-lambda-multipart-parser/parser"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/google/uuid"
)

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Adding header to make it work with localhost or another host and proxy request 
	// to void cors issues
	headers := make(map[string]string);

	headers["Access-Control-Allow-Origin"] = "*";
	headers["Access-Control-Allow-Methods"] = "OPTIONS,POST";
	headers["Access-Control-Allow-Headers"] = "Access-Control-Allow-Headers, Origin,Accept, X-Requested-With, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers, Access-Control-Allow-Origin";

	// handle pre-fligth request 
	if req.HTTPMethod == "OPTIONS" {

		// headers["Access-Control-Allow-Credentials"] = "true";

		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusOK,
			Headers: headers,
			Body: "Success",
		}, nil
	}
	// Parse the request.
	data, err := parser.Parse(req)
	
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Headers: headers,
			Body: "Failed to parse request",
		}, err
	}

	// TODO: Get the file content type instead of using the lambda parser mdoule

	// Attempt to read the file in form field 'file'.
	file, ok := data.File("file")

	if !ok {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Headers: headers,
			Body: "missing file",
		}, nil
	}

	// parse the body to get the image base64 string

	var boundary string;

	for k, v := range req.Headers {
		if strings.ToLower(k) == "content-type" {
			parts := strings.Split(v, "=")

			boundary = parts[1]
		}
	}

	for k, v := range req.Headers {
		if strings.ToLower(k) == "content-type" {
			log.Println(k)
			log.Println(v)

			_, params, err := mime.ParseMediaType(req.Headers[k])

			if err != nil {
				log.Println(err.Error())

				return events.APIGatewayProxyResponse{
					StatusCode: http.StatusBadRequest,
					Headers: headers,
					Body: "Failed to parse media type from header",
				}, nil
			} else {
				log.Println("parsed boundary", params["boundary"])
			}
		}
	}


	_, params, err := mime.ParseMediaType(req.Headers["Content-Type"])

	if err != nil {
		log.Println(err.Error())

		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Headers: headers,
			Body: "Failed to parse media type from header",
		}, nil
	} else {
		log.Println("parsed boundary", params["boundary"])
	}

	decodedBody, _ := base64.StdEncoding.DecodeString(req.Body)
	
	multipartReader := multipart.NewReader(strings.NewReader(string(decodedBody)), boundary)

	var imageFile []byte;

	for {
		part, err := multipartReader.NextPart();

		if err == io.EOF {
			break
		}
		
		if err != nil {
			break
		}
		
		defer part.Close()
	
		fileBytes, err := ioutil.ReadAll(part)

		detectedTypestring := strings.TrimSpace(http.DetectContentType(fileBytes))
		decodedTypestring := strings.TrimSpace(file.ContentType)

		if detectedTypestring == decodedTypestring {
			fmt.Println("image file type")
			imageFile = fileBytes;
		}

		if err != nil {
			break;
		}
	}

	log.Printf("File Type: %s\n", file.Type)
	log.Printf("Filename: %s\n", file.Filename)
	log.Printf("Content Type: %s\n", file.ContentType)

	// Upload to s3
	uuid := uuid.New()
	bucket := "gigslive-amp-dev"
	sourceFileName := strings.Split(file.ContentType, "/")
	fileType := sourceFileName[0]
	sourceFileFormatt := sourceFileName[1]

	// set the file name
	fileName := fileType + "_" + uuid.String() + "." + sourceFileFormatt;

	// Create a aws s3 uploader
	awsSession, err := session.NewSession(&aws.Config{
        Region: aws.String("us-east-1")},
    )

	if err != nil {
        return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Headers: headers,
			Body: "Failed to connect to aws s3",
		}, nil
    }

	s3Uploader := s3manager.NewUploader(awsSession)

	// Create a io.ReadSeeker buffer of file to upload

    detectedfileType := http.DetectContentType(file.Content)

	log.Printf("Detected file type is %s", detectedfileType)

	result, err := s3Uploader.Upload(&s3manager.UploadInput{
        Bucket: aws.String(bucket),
		// remove carriage return character from string 
        Key: aws.String(strings.TrimSpace(fileName)),
        Body: bytes.NewReader(imageFile),
		ACL: aws.String("public-read"),
		// ContentType: aws.String(file.ContentType),
    })

    if err != nil {
		log.Printf("Upload Failed %s\n", err.Error())

        return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Headers: headers,
			Body: "Failed to upload to s3",
		}, nil
    }

	response := fmt.Sprintf(`{ "s3_url": "%s" }`, result.Location)


	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: headers,
		Body: response,
	}, nil
}


func main() {
	// Make the handler available for Remote Procedure Call by AWS Lambda
	lambda.Start(handler)
}

