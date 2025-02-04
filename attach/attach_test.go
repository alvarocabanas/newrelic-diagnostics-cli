package attach

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/newrelic/newrelic-diagnostics-cli/config"
	"github.com/newrelic/newrelic-diagnostics-cli/helpers/httpHelper"
	"github.com/newrelic/newrelic-diagnostics-cli/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var testServer *httptest.Server

type MockGetReaderRet struct {
	byts *bytes.Reader
	err  error
}
type MockGetUrlsToReturnRet struct {
	url *string
	err error
}
type MockReturns struct {
	getFileSize     int64
	getReader       MockGetReaderRet
	getWrapper      httpHelper.RequestWrapper
	getUrlsToReturn MockGetUrlsToReturnRet
}

func setup() {
	testServer = httptest.NewServer((http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/success") {
			w.WriteHeader(200)
		}
		if strings.Contains(r.URL.Path, "/error") {

			w.WriteHeader(500)
		}
	})))
}

func teardown() {
	testServer.Close()
}

func Test_uploadFilesToAccount(t *testing.T) {
	setup()
	defer teardown()
	type args struct {
		filesToUpload []UploadFiles
		attachmentKey string
	}
	jsonfile := UploadFiles{
		Path:        "/",
		Filename:    "file1.json",
		NewFilename: "file1-timestamp.json",
		Filesize:    4,
		URL:         "",
		Key:         "testKey",
	}

	wantedUrl := "https://newrelic.com"
	tests := []struct {
		name        string
		args        args
		want        []string
		wantErr     bool
		mockReturns MockReturns
	}{
		{
			name: "Test successful uploadFilesToAccount",
			args: args{
				filesToUpload: []UploadFiles{jsonfile},
				attachmentKey: "testKey",
			},
			wantErr: false,
			want:    []string{wantedUrl},
			mockReturns: MockReturns{
				getFileSize: 4,
				getReader: MockGetReaderRet{
					byts: bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					err:  nil,
				},
				getWrapper: httpHelper.RequestWrapper{
					Method:         "POST",
					URL:            testServer.URL + "/success",
					Payload:        bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					Length:         4,
					TimeoutSeconds: awsUploadTimeoutSeconds,
					Headers:        map[string]string{"Attachment-Key": "123563454"},
				},
				getUrlsToReturn: MockGetUrlsToReturnRet{
					url: &wantedUrl,
					err: nil,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAttachDeps := new(mocks.MAttachDeps)
			mockAttachDeps.On("GetFileSize", mock.Anything).Return(tt.mockReturns.getFileSize)
			mockAttachDeps.On("GetReader", mock.Anything).Return(tt.mockReturns.getReader.byts, tt.mockReturns.getReader.err)
			mockAttachDeps.On("GetWrapper", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(tt.mockReturns.getWrapper)
			mockAttachDeps.On("GetUrlsToReturn", mock.Anything).Return(tt.mockReturns.getUrlsToReturn.url, tt.mockReturns.getUrlsToReturn.err)
			got, err := uploadFilesToAccount("upload_s3", tt.args.filesToUpload, tt.args.attachmentKey, mockAttachDeps)
			if (err != nil) != tt.wantErr {
				t.Errorf("uploadFilesToAccount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("uploadFilesToAccount() = %v, want %v", got, tt.want)
			}
		})
	}
}
func Test_uploadFile(t *testing.T) {
	setup()
	defer teardown()

	jsonfile := UploadFiles{
		Path:        "/",
		Filename:    "file1.json",
		NewFilename: "file1-timestamp.json",
		Filesize:    4,
		URL:         "",
		Key:         "testKey",
	}
	wantedUrl := "https://newrelic.com"

	type args struct {
		filesToUpload UploadFiles
		attachmentKey string
	}
	tests := []struct {
		name        string
		args        args
		wantErr     bool
		want        *string
		mockReturns MockReturns
	}{
		{
			name: "Test successful uploadFilesToAccount",
			args: args{
				filesToUpload: jsonfile,
				attachmentKey: "testKey",
			},
			wantErr: false,
			want:    &wantedUrl,
			mockReturns: MockReturns{
				getFileSize: 4,
				getReader: MockGetReaderRet{
					byts: bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					err:  nil,
				},
				getWrapper: httpHelper.RequestWrapper{
					Method:         "POST",
					URL:            testServer.URL + "/success",
					Payload:        bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					Length:         4,
					TimeoutSeconds: awsUploadTimeoutSeconds,
					Headers:        map[string]string{"Attachment-Key": "123563454"},
				},
				getUrlsToReturn: MockGetUrlsToReturnRet{
					url: &wantedUrl,
					err: nil,
				},
			},
		},
		{
			name: "Test with Reader Error",
			args: args{
				filesToUpload: jsonfile,
				attachmentKey: "testKey",
			},
			wantErr: true,
			want:    nil,
			mockReturns: MockReturns{
				getFileSize: 4,
				getReader: MockGetReaderRet{
					byts: nil,
					err:  errors.New("Error uploading at Reader"),
				},
				getWrapper: httpHelper.RequestWrapper{
					Method:         "POST",
					URL:            testServer.URL + "/success",
					Payload:        bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					Length:         4,
					TimeoutSeconds: awsUploadTimeoutSeconds,
					Headers:        map[string]string{"Attachment-Key": "123563454"},
				},
				getUrlsToReturn: MockGetUrlsToReturnRet{
					url: &wantedUrl,
					err: nil,
				},
			},
		},
		{
			name: "Test with non 200 status code",
			args: args{
				filesToUpload: jsonfile,
				attachmentKey: "testKey",
			},
			wantErr: true,
			want:    nil,
			mockReturns: MockReturns{
				getFileSize: 4,
				getReader: MockGetReaderRet{
					byts: bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					err:  nil,
				},
				getWrapper: httpHelper.RequestWrapper{
					Method:         "POST",
					URL:            testServer.URL + "/error",
					Payload:        bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					Length:         4,
					TimeoutSeconds: awsUploadTimeoutSeconds,
					Headers:        map[string]string{"Attachment-Key": "123563454"},
				},
				getUrlsToReturn: MockGetUrlsToReturnRet{
					url: &wantedUrl,
					err: nil,
				},
			},
		},
		{
			name: "Test with url error",
			args: args{
				filesToUpload: jsonfile,
				attachmentKey: "testKey",
			},
			wantErr: true,
			want:    nil,
			mockReturns: MockReturns{
				getFileSize: 4,
				getReader: MockGetReaderRet{
					byts: bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					err:  nil,
				},
				getWrapper: httpHelper.RequestWrapper{
					Method:         "POST",
					URL:            testServer.URL + "/success",
					Payload:        bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					Length:         4,
					TimeoutSeconds: awsUploadTimeoutSeconds,
					Headers:        map[string]string{"Attachment-Key": "123563454"},
				},
				getUrlsToReturn: MockGetUrlsToReturnRet{
					url: nil,
					err: errors.New("URL Error"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAttachDeps := new(mocks.MAttachDeps)
			mockAttachDeps.On("GetFileSize", mock.Anything).Return(tt.mockReturns.getFileSize)
			mockAttachDeps.On("GetReader", mock.Anything).Return(tt.mockReturns.getReader.byts, tt.mockReturns.getReader.err)
			mockAttachDeps.On("GetWrapper", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(tt.mockReturns.getWrapper)
			mockAttachDeps.On("GetUrlsToReturn", mock.Anything).Return(tt.mockReturns.getUrlsToReturn.url, tt.mockReturns.getUrlsToReturn.err)

			got, err := uploadFile("upload_s3", tt.args.filesToUpload, tt.args.attachmentKey, mockAttachDeps)
			if (err != nil) != tt.wantErr {
				t.Errorf("uploadFilesToAccount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("uploadFilesToAccount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAttachDeps_getAttachmentsEndpoint(t *testing.T) {
	tests := []struct {
		name                    string
		attachmentEndpoint      string
		flagsAttachmentEndpoint string
		want                    string
	}{
		{
			name:                    "Test GetAttachmentEndpoint none set",
			attachmentEndpoint:      "",
			flagsAttachmentEndpoint: "",
			want:                    "http://localhost:3000/attachments",
		},
		{
			name:                    "Test GetAttachmentEndpoint config.Flags.AttachmentEndpoint set",
			attachmentEndpoint:      "",
			flagsAttachmentEndpoint: "http://diag.datanerd.us/attachments",
			want:                    "http://diag.datanerd.us/attachments",
		},
		{
			name:                    "Test GetAttachmentEndpoint config.AttachmentEndpoint set",
			attachmentEndpoint:      "http://diag.datanerd.us/attachments",
			flagsAttachmentEndpoint: "",
			want:                    "http://diag.datanerd.us/attachments",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.Flags.AttachmentEndpoint = tt.flagsAttachmentEndpoint
			config.AttachmentEndpoint = tt.attachmentEndpoint
			if got := getAttachmentsEndpoint(); got != tt.want {
				t.Errorf("AttachDeps.GetAttachmentsEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAttachDeps_makeRequest(t *testing.T) {
	setup()
	defer teardown()
	type args struct {
		wrapper httpHelper.RequestWrapper
	}
	tests := []struct {
		name           string
		args           args
		wantStatusCode int
	}{
		{
			name: "Test MakeRequest success",
			args: args{
				wrapper: httpHelper.RequestWrapper{
					Method:         "POST",
					URL:            testServer.URL + "/success",
					Headers:        map[string]string{"Attachment-Key": "testKey"},
					Payload:        bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					Length:         4,
					TimeoutSeconds: 7200,
					BypassProxy:    false,
				},
			},
			wantStatusCode: 200,
		},
		{
			name: "Test MakeRequest fail",
			args: args{
				wrapper: httpHelper.RequestWrapper{
					Method:         "POST",
					URL:            testServer.URL + "/error",
					Headers:        map[string]string{"Attachment-Key": "testKey"},
					Payload:        bytes.NewReader([]byte{'m', 'o', 'c', 'k'}),
					Length:         4,
					TimeoutSeconds: 7200,
					BypassProxy:    false,
				},
			},
			wantStatusCode: 500,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := makeRequest(tt.args.wrapper)

			if got != nil {
				if !reflect.DeepEqual(got.StatusCode, tt.wantStatusCode) {
					t.Errorf("AttachDeps.MakeRequest() = %v, want %v", got, tt.wantStatusCode)
					return
				}
			}
		})
	}
}

// // Legacy tests below

type Client struct {
	cli *http.Client
	URL string
}

var _ = Describe("GetAttachmentEndpoints", func() {
	var (
		expectedResult string
	)
	Context("With no attachmentEndpointsSet", func() {
		BeforeEach(func() {
			config.Flags.AttachmentEndpoint = ""
			config.AttachmentEndpoint = ""
			expectedResult = "http://localhost:3000/attachments"
		})
		It("Should return default attachment endpoint (localhost)", func() {
			result := getAttachmentsEndpoint()
			Expect(result).To(Equal(expectedResult))
		})
	})
	Context("With flag attachment endpoints set", func() {
		BeforeEach(func() {
			config.Flags.AttachmentEndpoint = "http://diag.datanerd.us/attachments"
			config.AttachmentEndpoint = ""
			expectedResult = "http://diag.datanerd.us/attachments"
		})
		It("Should return flag attachment endpoint", func() {
			result := getAttachmentsEndpoint()
			Expect(result).To(Equal(expectedResult))
		})
	})
	Context("With attachment endpoint set", func() {
		BeforeEach(func() {
			config.Flags.AttachmentEndpoint = ""
			config.AttachmentEndpoint = "http://diag.datanerd.us/attachments"
			expectedResult = "http://diag.datanerd.us/attachments"
		})
		It("Should return attachment endpoint", func() {
			result := getAttachmentsEndpoint()
			Expect(result).To(Equal(expectedResult))
		})
	})

})

func TestBadAttachmentUploadURL(t *testing.T) {
	r := thisRouter(FailureGetRequest)

	s := httptest.NewServer(r)
	defer s.Close()

	thisClient := &Client{
		cli: s.Client(),
		URL: s.URL,
	}
	testAPIEndpoint := thisClient.URL + "/attachments/upload_url?attachment_key=badattachmentkey&filename=nrdiag-output.json&filesize=34256"

	req, err := http.NewRequest("GET", testAPIEndpoint, nil)
	if err != nil {
		panic(err)
	}

	res, err := thisClient.cli.Do(req)
	if err != nil {
		panic(err)
	}

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

func thisRouter(thisRequest func(w http.ResponseWriter, r *http.Request)) *mux.Router {
	r := mux.NewRouter().StrictSlash(true)
	r.Path("/attachments/upload_url").
		Queries("attachment_key", "{attachment_key:[a-zA-Z0-9]{32}|[a-zA-Z0-9]{40}}", "filename", "{filename:nrdiag-output.json|nrdiag-output.zip}", "filesize", "{filesize:[0-9]+}").
		HandlerFunc(thisRequest).
		Name("/attachments/upload_url")

	return r
}

func SuccessGetRequestAttachmentKey(w http.ResponseWriter, r *http.Request) {
	rawResponse, err := os.ReadFile("../mocks/hdash-response-good-attachment.json")
	if err != nil {
		panic(err)
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(rawResponse))
}
func SuccessGetRequestLicenseKey(w http.ResponseWriter, r *http.Request) {
	rawResponse, err := os.ReadFile("../mocks/hdash-response-good-license.json")
	if err != nil {
		panic(err)
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(rawResponse))
}

func FailureGetRequest(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "application/json")
}
