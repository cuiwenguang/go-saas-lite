package portal

import (
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets/portal.css
var portalCSS []byte

var pageNames = []string{"landing", "pricing", "signup", "done", "terms", "privacy"}

// signupForm 保留注册表单回显字段。
type signupForm struct {
	CompanyName string
	Phone       string
	Subdomain   string
}

type pageData struct {
	Title      string
	ConsoleURL string
	Plans      []PlanView
	Form       signupForm
	Error      string
	Message    string
	Code       string
	Pending    bool
	Year       int
}

// Handler 渲染门户页面并处理注册表单。
type Handler struct {
	service *Service
	opts    Options
	pages   map[string]*template.Template
	limiter *limiter
}

// NewHandler 创建门户 handler。
func NewHandler(service *Service, opts Options, client *redis.Client) (*Handler, error) {
	pages, err := loadPages()
	if err != nil {
		return nil, err
	}

	return &Handler{
		service: service,
		opts:    opts,
		pages:   pages,
		limiter: newLimiter(client),
	}, nil
}

func loadPages() (map[string]*template.Template, error) {
	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		tmpl, err := template.ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		pages[name] = tmpl
	}

	return pages, nil
}

// Register 挂载门户路由。
func (h *Handler) Register(site *gin.RouterGroup) {
	site.GET("/", h.landing)
	site.GET("/pricing", h.pricing)
	site.GET("/signup", h.signupPage)
	site.POST("/signup", h.signupSubmit)
	site.POST("/signup/otp", h.sendOtp)
	site.GET("/signup/done", h.done)
	site.GET("/login", h.login)
	site.GET("/terms", h.terms)
	site.GET("/privacy", h.privacy)
	site.GET("/portal.css", h.css)
}

func (h *Handler) landing(c *gin.Context) {
	h.render(c, "landing", http.StatusOK, pageData{
		Title:      "go-saas-lite",
		ConsoleURL: h.opts.ConsoleURL,
	})
}

func (h *Handler) pricing(c *gin.Context) {
	data := pageData{Title: "价格", ConsoleURL: h.opts.ConsoleURL}
	plans, err := h.service.Plans(c.Request.Context())
	if err != nil {
		data.Error = "套餐加载失败，请稍后再试"
	} else {
		data.Plans = plans
	}
	h.render(c, "pricing", http.StatusOK, data)
}

func (h *Handler) signupPage(c *gin.Context) {
	h.renderSignup(c, http.StatusOK, pageData{}, "")
}

func (h *Handler) signupSubmit(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)

	if !sameOrigin(c) {
		h.renderSignup(c, http.StatusForbidden, pageData{}, "请求来源不合法")
		return
	}
	if !h.allow(c, "signup", h.signupLimit()) {
		h.renderSignup(c, http.StatusTooManyRequests, pageData{}, "请求过于频繁，请稍后再试")
		return
	}
	if strings.TrimSpace(c.PostForm("website")) != "" {
		h.renderSignup(c, http.StatusBadRequest, pageData{}, "请求无效")
		return
	}

	form := signupForm{
		CompanyName: strings.TrimSpace(c.PostForm("company_name")),
		Phone:       strings.TrimSpace(c.PostForm("phone")),
		Subdomain:   strings.TrimSpace(c.PostForm("subdomain")),
	}

	result, err := h.service.Signup(c.Request.Context(), SignupInput{
		CompanyName: form.CompanyName,
		Phone:       form.Phone,
		Code:        strings.TrimSpace(c.PostForm("code")),
		Password:    c.PostForm("password"),
		Subdomain:   form.Subdomain,
		AcceptTerms: c.PostForm("accept_terms") == "on",
	})
	if err != nil {
		h.renderSignup(c, http.StatusBadRequest, pageData{Form: form}, apperr.From(err).Msg)
		return
	}

	pending := "0"
	if result.Pending {
		pending = "1"
	}
	c.Redirect(http.StatusSeeOther, "/signup/done?code="+url.QueryEscape(result.Code)+"&pending="+pending)
}

func (h *Handler) sendOtp(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)

	if !sameOrigin(c) {
		response.Fail(c, apperr.New(apperr.CodeForbidden, "请求来源不合法"))
		return
	}
	if !h.allow(c, "otp", h.signupLimit()*4) {
		response.Fail(c, apperr.New(apperr.CodeQuotaExceeded, "请求过于频繁，请稍后再试"))
		return
	}

	phone := strings.TrimSpace(c.PostForm("phone"))
	if phone == "" {
		var payload struct {
			Phone string `json:"phone"`
		}
		if err := c.ShouldBindJSON(&payload); err == nil {
			phone = strings.TrimSpace(payload.Phone)
		}
	}

	if err := h.service.SendSignupCode(c.Request.Context(), phone); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"sent": true})
}

func (h *Handler) done(c *gin.Context) {
	h.render(c, "done", http.StatusOK, pageData{
		Title:      "注册成功",
		ConsoleURL: h.opts.ConsoleURL,
		Code:       c.Query("code"),
		Pending:    c.Query("pending") == "1",
	})
}

func (h *Handler) login(c *gin.Context) {
	if h.opts.ConsoleURL == "" {
		h.render(c, "landing", http.StatusOK, pageData{
			Title:   "go-saas-lite",
			Message: "控制台地址未配置",
		})
		return
	}

	c.Redirect(http.StatusFound, h.opts.ConsoleURL)
}

func (h *Handler) terms(c *gin.Context) {
	h.render(c, "terms", http.StatusOK, pageData{Title: "服务条款"})
}

func (h *Handler) privacy(c *gin.Context) {
	h.render(c, "privacy", http.StatusOK, pageData{Title: "隐私政策"})
}

func (h *Handler) css(c *gin.Context) {
	c.Data(http.StatusOK, "text/css; charset=utf-8", portalCSS)
}

func (h *Handler) renderSignup(c *gin.Context, status int, data pageData, message string) {
	data.Title = "免费注册"
	data.ConsoleURL = h.opts.ConsoleURL
	data.Error = message
	if data.Plans == nil {
		if plans, err := h.service.Plans(c.Request.Context()); err == nil {
			data.Plans = plans
		}
	}
	h.render(c, "signup", status, data)
}

func (h *Handler) render(c *gin.Context, page string, status int, data pageData) {
	tmpl, ok := h.pages[page]
	if !ok {
		c.String(http.StatusInternalServerError, "page not found")
		return
	}

	if data.Year == 0 {
		data.Year = time.Now().Year()
	}
	if data.ConsoleURL == "" {
		data.ConsoleURL = h.opts.ConsoleURL
	}

	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(c.Writer, "layout", data); err != nil {
		c.String(http.StatusInternalServerError, "render failed")
	}
}

func (h *Handler) signupLimit() int {
	if h.opts.SignupRatePerHour > 0 {
		return h.opts.SignupRatePerHour
	}
	return 5
}

func (h *Handler) allow(c *gin.Context, kind string, limit int) bool {
	return h.limiter.Allow(c.Request.Context(), kind+":"+c.ClientIP(), limit, time.Hour)
}

// sameOrigin 只允许同源表单提交；非浏览器请求（无 Origin）放行。
func sameOrigin(c *gin.Context) bool {
	origin := c.GetHeader("Origin")
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	return strings.EqualFold(parsed.Host, c.Request.Host)
}
