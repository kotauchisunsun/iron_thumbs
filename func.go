package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"github.com/nfnt/resize"
	"image/png"
)

func download_image(filename string, sess *session.Session, bucket string, key string) error {
	file, err := os.Create(filename)

	if err != nil {
		return err
	}
	defer file.Close()

	downloader := s3manager.NewDownloader(sess)
	_, download_err := downloader.Download(
		file,
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})

	if download_err != nil {
		return download_err
	}

	return nil
}

func make_thumb_name(filename string, size int) string {
	return fmt.Sprintf("%s%d", filename, size)
}

func download_thumb(sess *session.Session, bucket string, key string) error {
	cache_file, err := os.Create("/tmp/thumb_cache")
	if err != nil {
		return err
	}

	downloader := s3manager.NewDownloader(sess)
	_, download_err := downloader.Download(
		cache_file,
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})

	if download_err != nil {
		return download_err
	}

	_, copy_err := io.Copy(os.Stdout, cache_file)
	if copy_err != nil {
		return copy_err
	}

	return nil
}

func upload_thumb(filename string, sess *session.Session, bucket string, key string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	uploader := s3manager.NewUploader(sess)

	_, upload_err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if upload_err != nil {
		return upload_err
	}

	return nil
}

func make_thumb(filename string, size int, sess *session.Session, image_bucket string, thumb_bucket string) error {
	thumb_name := make_thumb_name(filename, size)
	tmp_filename := "/tmp/" + filename
	resized_filename := "/tmp/resized_" + filename

	key := filename

	log.Println("call download_image")
	err := download_image(tmp_filename, sess, image_bucket, key)
	log.Println("finish download_image")

	if err != nil {
		return err
	}

	log.Println("make thumbs")
	file, err := os.Open(tmp_filename)
	if err != nil {
		return err
	}

	img, err := png.Decode(file)
	if err != nil {
		return err
	}
	file.Close()

	m := resize.Resize(uint(size), 0, img, resize.Lanczos3)

	out, err := os.Create(resized_filename)
	if err != nil {
		return err
	}

	png.Encode(out, m)
	out.Close()
	log.Println("finish thumbs")

	upload_fin := make(chan bool)
	go func() {
		log.Println("call upload_image")

		upload_error := upload_thumb(resized_filename, sess, thumb_bucket, thumb_name)

		if upload_error != nil {
			log.Println("error upload_image")
		} else {
			log.Println("finish upload_image")
		}

		upload_fin <- false
	}()

	thumb_file, thumb_err := os.Open(resized_filename)
	if thumb_err != nil {
		return err
	}

	_, copy_err := io.Copy(os.Stdout, thumb_file)
	if copy_err != nil {
		return copy_err
	}

	<-upload_fin

	return nil
}

func main() {
	ACCESS_KEY := os.Getenv("S3_ACCESS_KEY")
	SECRET_KEY := os.Getenv("S3_SECRET_KEY")
	ENDPOINT := os.Getenv("S3_ENDPOINT")
	REGION := os.Getenv("S3_REGION")
	IMAGE_BUCKET := os.Getenv("IMAGE_BUCKET")
	THUMB_BUCKET := os.Getenv("THUMB_BUCKET")

	URL := os.Getenv("REQUEST_URL")

	u, _ := url.Parse(URL)
	q, _ := url.ParseQuery(u.RawQuery)

	filename := q["filename"][0]
	size, err := strconv.Atoi(q["size"][0])

	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}

	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(ACCESS_KEY, SECRET_KEY, ""),
		Endpoint:         aws.String(ENDPOINT),
		Region:           aws.String(REGION),
		DisableSSL:       aws.Bool(true),
		S3ForcePathStyle: aws.Bool(true),
	}

	sess := session.New(s3Config)

	thumb_name := make_thumb_name(filename, size)

	log.Println("call download_thumb")
	s3_thumb_err := download_thumb(sess, THUMB_BUCKET, thumb_name)

	if s3_thumb_err != nil {
		log.Println(s3_thumb_err)
		log.Println("call make_thumb")
		thumb_err := make_thumb(filename, size, sess, IMAGE_BUCKET, THUMB_BUCKET)
		log.Println("finish make_thumb")

		if thumb_err != nil {
			fmt.Printf("%s\n", thumb_err)
			return
		}
	}

}
