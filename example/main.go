package main

import (
	"github.com/aluka-7/configuration"
	"github.com/aluka-7/web"
	"github.com/labstack/echo/v4"
)

func App(conf configuration.Configuration) {
	web.App(func(eng *echo.Echo) {
		eng.Static("/static", "static")
		eng.Renderer = web.NewWebAppTemplate(web.RenderOptions{
			Directory: "views",
			Layout:    "common/layout",
			// 追加的 Content-Type 头信息，默认为 "UTF-8"
			Charset: "UTF-8",
			// 允许输出格式为 XHTML 而不是 HTML，默认为 "text/html"
			HTMLContentType: "text/html",
		})
		eng.GET("/", func(c echo.Context) error {
			return c.Render(200, "index", "World")
		})
	}, "9999", conf)
}

func main() {
	conf := configuration.DefaultEngine()
	App(conf)
}
