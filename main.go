package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"time"

	"github.com/gin-gonic/gin"
	"github.com/abadojack/whatlanggo"
)

func main() {
	r := gin.Default()

	r.GET("/api/v1/crawling/fb", func(r *gin.Context) {
		av := r.Query("av")
		user := r.Query("user")
		doc_id := r.Query("doc_id")
		cookie := r.Query("cookie")
		keywords := strings.Split(r.Query("keywords"), ",")
		videos, creators, err := DiscoverCli(av, user, doc_id, cookie, keywords)
		if err != nil {
			r.JSON(http.StatusOK, gin.H{
				"error": err.Error(),
			})
		}
		r.JSON(http.StatusOK, gin.H{
			"videos": videos,
			"creators": creators,
		})

	})

	r.Run(":5000")
}

type user struct {
	ID             string
	Name		   string
	Url			   string
	IsVerified     bool
}

type userStats struct {
	DiggCount      int
	FollowerCount  int
	FollowingCount int
	Heart          int
	HeartCount     int
	VideoCount     int
}

type videoStats struct {
	CommentCount int64
	DiggCount    int64
	PlayCount    int64
	ShareCount   int64
}

type video struct {
	ID          string
	Url			string
	Author      *user
	AuthorStats *userStats
	CreateTime  int64
	Desc        string
	Stats       *videoStats
	Video       struct {
		Cover    string
		Duration int64
	}
	UpdatedAt   time.Time
}

func requestAPI(client *http.Client, av, _user, fbdtsg, variables, doc_id, keyword, cookie string) (videos []*Video, users []*Creator, err error) {

	fbdtsg = url.QueryEscape(fbdtsg)
	variables = url.QueryEscape(variables)

	var data = strings.NewReader(fmt.Sprintf(`av=%s&__user=%s&fb_dtsg=%s&variables=%s&doc_id=%s`, av, _user, fbdtsg, variables, doc_id))

	req, err := http.NewRequest("POST", "https://www.facebook.com/api/graphql/", data)
	if err != nil {
		fmt.Println(err)
		return nil, nil, err
	}
	req.Header.Set("authority", "www.facebook.com")
	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("cookie", cookie)
	req.Header.Set("origin", "https://www.facebook.com")
	req.Header.Set("referer", "https://www.facebook.com")
	req.Header.Set("sec-ch-prefers-color-scheme", "light")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="113", "Chromium";v="113", "Not-A.Brand";v="24"`)
	req.Header.Set("sec-ch-ua-full-version-list", `"Google Chrome";v="113.0.5672.63", "Chromium";v="113.0.5672.63", "Not-A.Brand";v="24.0.0.0"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Linux"`)
	req.Header.Set("sec-ch-ua-platform-version", `"6.2.13"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")
	req.Header.Set("viewport-width", "1048")
	req.Header.Set("x-asbd-id", "198387")
	req.Header.Set("x-fb-friendly-name", "SearchCometResultsInitialResultsQuery")
	req.Header.Set("x-fb-lsd", "aODIIFYvpgJs6736qsBMP3")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return nil, nil, err
	}
	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return nil, nil, err
	}
	var bodyJson map[string]interface{}
	err = json.Unmarshal(bodyText, &bodyJson)
	if err != nil {
		fmt.Println(err)
		return nil, nil, err
	}
	if _, ok := bodyJson["data"].(map[string]interface{}); !ok {
		if err_, ok := bodyJson["errors"].([]interface{}); ok {
			fmt.Println(err_)
			if message, ok := err_[0].(map[string]interface{})["message"].(string); ok {
				return nil, nil, errors.New(message)
			}
		}
	}
	if bodyJson, ok := bodyJson["data"].(map[string]interface{}); ok {
		if viewer, ok := bodyJson["viewer"].(map[string]interface{}); ok {
			if _, ok := viewer["call_blocked_until"]; ok {
				return nil, nil, errors.New("call_blocked_until")
			}
		}
	}

	if bodyJson, ok := bodyJson["data"].(map[string]interface{}); ok {
		if serpResponse, ok := bodyJson["serpResponse"].(map[string]interface{}); ok {
			if results, ok := serpResponse["results"].(map[string]interface{}); ok {
				if edges, ok := results["edges"].([]interface{}); ok {
					var video video
					var videoMetadataModel interface{}
					var viewModel interface{}
					var relativeTimeString string
					var video_broadcast_status interface{}
					for _, edge := range edges {
						viewModel = edge.(map[string]interface{})["relay_rendering_strategy"].(map[string]interface{})["view_model"]
						videoMetadataModel = viewModel.(map[string]interface{})["video_metadata_model"].(map[string]interface{})
						video.ID = videoMetadataModel.(map[string]interface{})["video"].(map[string]interface{})["id"].(string)
						video.Author = &user{}
						video.Author.ID = videoMetadataModel.(map[string]interface{})["video_owner_profile"].(map[string]interface{})["id"].(string)
						video.Author.Url = "https://www.facebook.com/" + video.Author.ID
						video.Author.Name = videoMetadataModel.(map[string]interface{})["video_owner_profile"].(map[string]interface{})["name"].(string)
						video.Url = video.Author.Url + "/videos/" + video.ID
						relativeTimeString = videoMetadataModel.(map[string]interface{})["relative_time_string"].(string)
						if !strings.Contains(relativeTimeString, " · ") {
							continue
						}
						video_broadcast_status = videoMetadataModel.(map[string]interface{})["video_broadcast_status"]
						video.Stats = &videoStats{}
						switch video_broadcast_status {
						case "VOD_READY":
							video.CreateTime = time.Now().Unix()
							viewsStr := strings.Split(relativeTimeString, " · ")[0]
							viewsInt, _ := strconv.Atoi(strings.Split(viewsStr, " ")[0])
							video.Stats.PlayCount = int64(viewsInt)
						case "LIVE":
							continue
						case nil:
							video.CreateTime, video.Stats.PlayCount = extractTimeAndViews(relativeTimeString)
						default:
							video.CreateTime, video.Stats.PlayCount = extractTimeAndViews(relativeTimeString)
						}
						video.Desc = videoMetadataModel.(map[string]interface{})["title"].(string)
						video.Video.Cover = viewModel.(map[string]interface{})["video_thumbnail_model"].(map[string]interface{})["thumbnail_image"].(map[string]interface{})["uri"].(string)
						video.Video.Duration = int64(convertToSeconds(viewModel.(map[string]interface{})["video_thumbnail_model"].(map[string]interface{})["video_duration_text"].(string)))
						followersCount, con := getFollowerAndAbout(video.Author.ID)
						video.AuthorStats = &userStats{}
						video.AuthorStats.FollowerCount = int(followersCount)
						videos = append(videos, processVideo(&video, keyword))
						users = append(users, processUser(&video, con, keyword))
					}
					return videos, users, nil
				}
			}
		}
	}	
	return videos, users, errors.New("undefined error")
}

func convertToSeconds(timeStr string) int64 {
	var timeArr []string
	if len(timeStr) >= 4 {
		timeArr = strings.Split(timeStr, ":")
	}
	var timeIntArr []int
	for _, time := range timeArr {
		timeInt, _ := strconv.Atoi(time)
		timeIntArr = append(timeIntArr, timeInt)
	}
	if len(timeIntArr) == 2 {
		return int64(timeIntArr[0]*60 + timeIntArr[1])
	} else {
		return int64(timeIntArr[0]*3600 + timeIntArr[1]*60 + timeIntArr[2])
	}
}

func extractTimeAndViews(timeAndViews string) (int64, int64) {
	timeAndViewsArr := strings.Split(timeAndViews, " · ")
	timeStr := timeAndViewsArr[0]
	viewsStr := timeAndViewsArr[1]
	// extract time
	var timeInt64 int64
	if strings.Contains(timeStr, "giờ") {
		timeInt, _ := strconv.Atoi(strings.Split(timeStr, " ")[0])
		timeInt64 = time.Now().Unix() - int64(timeInt*3600)
	} else if strings.Contains(timeStr, "phút") {
		timeInt, _ := strconv.Atoi(strings.Split(timeStr, " ")[0])
		timeInt64 = time.Now().Unix() - int64(timeInt*60)
	} else if strings.Contains(timeStr, "giây") {
		timeInt, _ := strconv.Atoi(strings.Split(timeStr, " ")[0])
		timeInt64 = time.Now().Unix() - int64(timeInt)
	} else if strings.Contains(timeStr, "day") {
		timeInt, _ := strconv.Atoi(strings.Split(timeStr, " ")[0])
		timeInt64 = time.Now().Unix() - int64(timeInt*3600*24)
	} else if strings.Contains(timeStr, "week") {
		timeInt, _ := strconv.Atoi(strings.Split(timeStr, " ")[0])
		timeInt64 = time.Now().Unix() - int64(timeInt*3600*24*7)
	} else if strings.Contains(timeStr, "month") {
		timeInt, _ := strconv.Atoi(strings.Split(timeStr, " ")[0])
		timeInt64 = time.Now().Unix() - int64(timeInt*3600*24*30)
	} else if strings.Contains(timeStr, "year") {
		timeInt, _ := strconv.Atoi(strings.Split(timeStr, " ")[0])
		timeInt64 = time.Now().Unix() - int64(timeInt*3600*24*365)
	}
	// extract views
	var viewsInt64 int64
	if strings.Contains(viewsStr, "K") {
		viewsInt, _ := strconv.Atoi(strings.Split(viewsStr, "K")[0])
		viewsInt64 = int64(viewsInt * 1000)
	} else if strings.Contains(viewsStr, "M") {
		viewsInt, _ := strconv.Atoi(strings.Split(viewsStr, "M")[0])
		viewsInt64 = int64(viewsInt * 1000000)
	} else {
		viewsInt, _ := strconv.Atoi(strings.Split(viewsStr, " ")[0])
		viewsInt64 = int64(viewsInt)
	}

	return timeInt64, viewsInt64
}

func  discoverVideos(av, _user, doc_id, keyword, dtsg, cookie string) ([]*Video, []*Creator, error) {
	var variables = `{"count":5,"allow_streaming":false,"args":{"callsite":"COMET_GLOBAL_SEARCH","config":{"exact_match":false,"high_confidence_config":null,"intercept_config":null,"sts_disambiguation":null,"watch_config":null},"context":{"bsid":"abc","tsid":"0.28451313886234786"},"experience":{"encoded_server_defined_params":null,"fbid":null,"type":"VIDEOS_TAB"},"filters":["{\"name\":\"creation_time\",\"args\":\"{\\\"start_year\\\":\\\"startYear\\\",\\\"start_month\\\":\\\"startMonth\\\",\\\"end_year\\\":\\\"startYear\\\",\\\"end_month\\\":\\\"startMonth\\\",\\\"start_day\\\":\\\"startDay\\\",\\\"end_day\\\":\\\"startDay\\\"}\"}"],"text":"keyword"}}`
	thisYear, thisMonth, thisDay := time.Now().Date()
	startYear := strconv.Itoa(thisYear)
	startMonth := startYear + "-" + strconv.Itoa(int(thisMonth))
	startDay := startMonth + "-" + strconv.Itoa(thisDay)
	variables = strings.ReplaceAll(variables, "startYear", startYear)
	variables = strings.ReplaceAll(variables, "startMonth", startMonth)
	variables = strings.ReplaceAll(variables, "startDay", startDay)
	variables = strings.ReplaceAll(variables, "keyword", keyword)
	videos, users, err := requestAPI(&http.Client{}, av , _user , dtsg, variables, doc_id, keyword, cookie)
	if err != nil {
		return nil, nil, err
	}
	return videos, users, nil
}

func DiscoverCli(av, _user, doc_id, cookie string, keywords []string) ([]*Video, []*Creator, error){

	dtsg := getDTSG(cookie, 0)
	failedKeywords := []string{}
	errList := []string{}
	videos := []*Video{}
	creators := []*Creator{}
	for _, keyword := range keywords {
		v, u, err := discoverVideos(av, _user, doc_id, keyword, dtsg, cookie)
		if err != nil {
			return nil, nil, err
		}
		videos = append(videos, v...)
		creators = append(creators, u...)
		if err != nil {
			failedKeywords = append(failedKeywords, keyword)
			if !IsInArray(errList, err.Error()[0:10]) {
				errList = append(errList, err.Error()[0:10])
			}
			log.Printf("discover videos: %v, keyword: %v", err, keyword)
			continue
		}
	}
	if len(errList) > 0 {
	}
	return videos, creators, nil
}


func processVideo(videos *video, keyword string) (video *Video) {
	v := newVideo(videos, keyword)
	return v
}

func processUser(video *video, con *Contact, keyword string) *Creator {
	c := newCreator(video.Author, video.AuthorStats, keyword)
	if c.Insights != nil && c.Insights.FollowCount < 1000 {
		return nil
	}
	c.Contact = con
	if con != nil {
		c.About = con.Category + " " + con.Phone + " " + con.Email
	}
	return c
}

func newVideo(v *video, keyword string) *Video {
	c := newCreator(v.Author, v.AuthorStats, keyword)
	now := c.Timestamp
	ts := time.Unix(v.CreateTime, 0)
	vv := &Video{
		ID:          v.ID,
		Desc:        v.Desc,
		Title:       keyword,
		CreatorID:   v.Author.ID,
		Platform:    PlatformFBVideo,
		Creator:     c,
		Picture:     v.Video.Cover,
		Link:        "/" + v.Author.ID + "/" + "videos" + "/" + v.ID,
		Length:      v.Video.Duration,
		Lang:        DetectLanguage(v.Desc),
		PublishedAt: &ts,
		UpdatedAt:   now,
		Timestamp:   now,
	}

	if vv.Lang == "vi" {
		c.Country = "VN"
	}

	if v.Stats != nil {
		vv.Insights = &VideoInsights{
			Views:     v.Stats.PlayCount,
			Timestamp: now,
		}
	}

	return vv
}

func newCreator(u *user, s *userStats, keyword string) *Creator {
	now := time.Now()
	c := &Creator{
		ID:        u.ID,
		Title:	   keyword,
		Platform:  PlatformFBVideo,
		Name:      u.Name,
		Link:      u.Url,
		Timestamp: now,
		UpdatedAt: now,
	}

	c.Country = "VN"

	if u.IsVerified {
		c.AddExtraData("verified", true)
	}

	if s != nil {
		c.Insights = &CreatorInsights{
			FollowCount: int64(s.FollowerCount),
			VideoCount:  int64(s.VideoCount),
			LikeCount:   int64(s.HeartCount),
			Timestamp:   now,
		}
	}

	return c
}


func getFollowerAndAbout(page string) (int64, *Contact) {
	client := &http.Client{}
	url := fmt.Sprintf("https://www.facebook.com/%s/about", page)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		return 0, nil
	}
	req.Header.Set("authority", "www.facebook.com")
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("cache-control", "max-age=0")
	req.Header.Set("cookie", "datr=q6ldZMdfrug75WWOGKKF4dyF; fr=07Ghb3VlIumdXU65x..BkXaoG.Ko.AAA.0.0.BkXaqA.AWVjwPUoL28; wd=441x946")
	req.Header.Set("sec-ch-prefers-color-scheme", "light")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="113", "Chromium";v="113", "Not-A.Brand";v="24"`)
	req.Header.Set("sec-ch-ua-full-version-list", `"Google Chrome";v="113.0.5672.63", "Chromium";v="113.0.5672.63", "Not-A.Brand";v="24.0.0.0"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Linux"`)
	req.Header.Set("sec-ch-ua-platform-version", `"6.2.13"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("upgrade-insecure-requests", "1")
	req.Header.Set("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")
	req.Header.Set("viewport-width", "441")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return 0, nil
	}
	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return 0, nil
	}
	html := string(bodyText)
	var followers int64

	// Extract followers using regular expression
	if strings.Contains(html, "follower_count") {
		re := regexp.MustCompile(`"follower_count":(\d+)`)
		match := re.FindStringSubmatch(html)
		if len(match) > 1 {
			count := match[1]
			followers = convertStringToInt(count)
			fmt.Printf("Follower count: %d\n", followers)
		} else {
			fmt.Println("Follower count not found in the HTML.")
		}
	} else {
		re := regexp.MustCompile(`"text":"([^"]+ followers)"`)
		match := re.FindStringSubmatch(html)
		if len(match) > 1 {
			count := strings.Split(match[1], " ")[0]
			followers = convertStringToInt(count)
			fmt.Printf("Follower count: %d\n", followers)
		} else {
			fmt.Println("Text not found in the HTML.")
		}
	}

	re := regexp.MustCompile(`"text":"([^"]+)"},"field_type":("[^"]+")`)

    match_ := re.FindAllStringSubmatch(html, -1)
	con := &Contact{}
	if len(match_) > 0 {
		for _, m := range match_ {
			if m[2] == "\"profile_phone\"" {
				con.Phone = m[1]
			}
			if m[2] == "\"profile_email\"" {
				con.Email = m[1]
				con.Email = strings.ReplaceAll(con.Email, `\u0040`, "@")
			}
			if m[2] == "\"website\"" {
				con.Website = m[1]
				// decode unicode string
				con.Website = strings.ReplaceAll(con.Website, `\`, "")
			}
			if m[2] == "\"category\"" {
				con.Category = m[1]
			}
		}
	}

	return followers, con
}

func convertStringToInt(s string) int64 {
	var result int64
	if strings.Contains(s, "K") {
		s = strings.Replace(s, "K", "", 1)
		sFloat, _ := strconv.ParseFloat(s, 64)
		result = int64(float64(sFloat) * 1000)
	} else if strings.Contains(s, "M") {
		sFloat, _ := strconv.ParseFloat(s, 64)
		result = int64(float64(sFloat) * 1000000)
	} else if strings.Contains(s, "B") {
		s = strings.Replace(s, "M", "", 1)
		sFloat, _ := strconv.ParseFloat(s, 64)
		result = int64(float64(sFloat) * 1000000000)
	} else {
		sInt, _ := strconv.Atoi(s)
		result = int64(sInt)
	}
	return result
}

func getDTSG(cookie string, try int64) string {
	if try == 5 {
		log.Println("Token(dtsg) is not found in the HTML.")
		return ""
	}
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://www.facebook.com/me/about/", nil)
	if err != nil {
		fmt.Println(err)
	}
	req.Header.Set("authority", "www.facebook.com")
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("cache-control", "max-age=0")
	req.Header.Set("cookie", cookie)
	req.Header.Set("sec-ch-prefers-color-scheme", "light")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="113", "Chromium";v="113", "Not-A.Brand";v="24"`)
	req.Header.Set("sec-ch-ua-full-version-list", `"Google Chrome";v="113.0.5672.63", "Chromium";v="113.0.5672.63", "Not-A.Brand";v="24.0.0.0"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Linux"`)
	req.Header.Set("sec-ch-ua-platform-version", `"6.3.1"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("upgrade-insecure-requests", "1")
	req.Header.Set("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")
	req.Header.Set("viewport-width", "846")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	// Extract the token using regular expression
	html := string(bodyText)
	re := regexp.MustCompile(`"token":"([^"]+)"`)
	match := re.FindStringSubmatch(html)
	if len(match) > 1 {
		token := match[1]
		fmt.Println("Token:", token)
		return token
	} else {
		fmt.Println("Token not found in the HTML.")
		getDTSG(cookie, try+1)
	}
	return ""
}

func IsInArray(errList []string, err string) bool {
	for _, e := range errList {
		if e == err {
			return true
		}
	}
	return false
}


func DetectLanguage(s string) string {
	if s == "" {
		return ""
	}

	s = strings.ReplaceAll(s, "\n", " ")
	inf := whatlanggo.Detect(s)
	if inf.Confidence >= 0.9 {
		return inf.Lang.Iso6391()
	}

	words := strings.Split(s, " ")
	var total, vcount int
	const vchars = "ăâáắấàằầảẳẩãẵẫạặậđêéếèềẻểẽễẹệíìỉĩịôơóốớòồờỏổởõỗỡọộợưúứùừủửũữụựýỳỷỹỵ"
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		total++
		if strings.ContainsAny(w, vchars) {
			vcount++
		}
	}
	if total > 0 && float64(vcount)/float64(total) > 0.3 {
		return "vi"
	}

	if inf.Confidence >= 0.3 {
		return inf.Lang.Iso6391()
	}

	return ""
}

type DiscoverRequest struct {
	Av    string   `json:"av"`
	User  string   `json:"user"`
	DocID string   `json:"doc_id"`
	Cookie string  `json:"cookie"`
	Keywords []string `json:"keywords"`
}

type platform string

const (
	PlatformNimoTV   platform = "NIMOTV"
	PlatformCubeTV   platform = "CUBETV"
	PlatformFBGaming platform = "FB_GAMING"
	PlatformFBVideo  platform = "FB_VIDEO"
	PlatformNonolive platform = "NONOLIVE"
	PlatformBigoTV   platform = "BIGOTV"
	PlatformYoutube  platform = "YOUTUBE"
	PlatformBooyah   platform = "BOOYAH"
	PlatformTikTok   platform = "TIKTOK"
)


type Contact struct {
	Email    string `bson:"email,omitempty" json:"email,omitempty"`
	Phone    string `bson:"phone,omitempty" json:"phone,omitempty"`
	Website  string `bson:"website,omitempty" json:"website,omitempty"`
	Category string `bson:"category,omitempty" json:"category,omitempty"`
	Status   string `valid:"oneof=POTENTIAL JOINED CANCELED -"`
	Rate     int    `valid:"min=0,max=10"`
}

type Video struct {
	ID           string         `bson:"_id" json:"id"`
	CreatorID    string         `bson:"creatorId"`
	Creator      *Creator       `json:"creator"`
	Title        string         `json:"title"`
	Desc         string         `json:"desc"`
	Picture      string         `json:"picture"`
	Link         string         `json:"link"`
	Platform     platform       `bson:"-"`
	Length       int64          `json:"length"` // seconds
	PeakCCV      int            `bson:"peakCCV" json:"peakCCV"`
	CCV          int            `bson:"ccv" json:"ccv"`
	AvgCCV       float64        `bson:"avgCCV" json:"avgCCV"`
	Segments     []VideoSegment `bson:",omitempty" json:"-"`
	SegmentCount int            `bson:"segmentCount" json:"-"`
	IsLive       bool           `bson:"live" json:"live"` // is living
	IsUploaded   bool           `bson:"isUploaded"`       // not a broadcast video
	Lang         string         `json:"lang"`
	PublishedAt  *time.Time     `bson:"publishedAt,omitempty"`
	Categories   []Category     `bson:",omitempty" json:"categories,omitempty"`
	UpdatedAt    time.Time      `bson:"updatedAt" json:"updatedAt"`
	Timestamp    time.Time      `bson:"ts" json:"ts"`
	Insights     *VideoInsights `bson:",omitempty"`
	IsShort      bool           `bson:"isShort"`
}

type VideoInsights struct {
	Views     int64
	Likes     int64
	Dislikes  int64
	Comments  int64
	Shares    int64
	Timestamp time.Time `bson:"ts"`
}


type Category struct {
	ID       string    `bson:"_id" json:"id"`
	Name     string    `json:"name,omitempty"`
	Tags     []string  `json:"tags,omitempty"` // gaming, mobile, pc, ps4, liveshow, ...
	Keywords []Keyword `json:"keywords,omitempty"`
}

type VideoSegment struct {
	CCV       int       `json:"ccv"`
	Length    int64     `json:"length"` // length = 0 mean unknown, video length = last segment ts - first segment ts
	Live      bool      `json:"live"`
	Timestamp time.Time `bson:"ts" json:"ts"`
}

type Keyword struct {
	ID         string `bson:"_id" json:"id"`
	Name       string `bson:"name" json:"name,omitempty"`
	Platform   string `bson:"platform,omitempty" json:"platform,omitempty"`
	CategoryID string `bson:"category_id" json:"category_id,omitempty"`
}

type Creator struct {
	ID         string `bson:"_id" json:"id"`
	CreatorID  string `bson:"creatorId" json:"creatorId"`
	Title      string `bson:"title" json:"title"`
	Name       string `bson:"name" json:"name"`
	Picture    string `bson:"picture" json:"picture"`
	Follows    int    `bson:"follows" json:"follows"`
	VideoCount int    `bson:"videoCount" json:"videoCount"`
	ViewCount  int    `bson:"viewCount" json:"viewCount"`
	Link       string `bson:"link" json:"link"`
	Country    string `bson:"country" json:"country"`
	Platform   platform `bson:"-"`
	About      string   `bson:"about,omitempty" json:"about,omitempty"`
	Tags       []string
	ExtraData  map[string]interface{} `bson:"extra,omitempty"`
	UpdatedAt  time.Time              `bson:"updatedAt" json:"updatedAt"`
	CreatedAt  *time.Time             `bson:"createdAt" json:"createdAt,omitempty"`
	Timestamp  time.Time              `bson:"ts" json:"ts"`
	Insights   *CreatorInsights       `bson:",omitempty"`
	Contact    *Contact               `bson:"contact,omitempty" json:"contact,omitempty"`
}

type CreatorInsights struct {
	FollowCount  int64     `bson:"followCount"`
	VideoCount   int64     `bson:"videoCount"`
	ViewCount    int64     `bson:"viewCount"`
	CommentCount int64     `bson:"commentCount"`
	LikeCount    int64     `bson:"likeCount"`
	Timestamp    time.Time `bson:"ts"`
}

func (s *Creator) AddExtraData(key string, data interface{}) {
	if s.ExtraData == nil {
		s.ExtraData = map[string]interface{}{}
	}
	s.ExtraData[key] = data
}
