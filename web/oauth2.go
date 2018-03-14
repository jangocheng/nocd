/*
 * Copyright (c) 2018, 奶爸<1@5.nu>
 * All rights reserved.
 */

package web

import (
	"fmt"
	"time"
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"github.com/jinzhu/gorm"
	"github.com/gin-contrib/sessions"
	"github.com/google/go-github/github"
	"git.cm/naiba/com"
	"git.cm/naiba/gocd"
)

func serveOauth2(r *gin.Engine) {
	oauth2router := r.Group("/oauth2")
	oauth2router.Use(filterMiddleware(filterOption{Guest: true}))
	{
		oauth2router.POST("/login", func(c *gin.Context) {
			session := sessions.Default(c)
			oauthToken := com.RandString(18)
			session.Set("oauth_token", oauthToken)
			session.Save()
			c.Redirect(http.StatusMovedPermanently, oauthConf.AuthCodeURL(oauthToken, oauth2.AccessTypeOnline))
		})

		oauth2router.GET("/callback", func(c *gin.Context) {
			type oauthCallback struct {
				State       string `form:"state"`
				RedirectUri string `form:"redirect_uri"`
				Code        string `form:"code"`
			}
			var call oauthCallback
			if err := c.ShouldBindQuery(&call); err != nil {
				c.String(400, "回调参数有误")
				return
			}
			// delete oauth_token
			session := sessions.Default(c)
			if session.Get("oauth_token").(string) != call.State {
				c.String(400, "登陆未授权，请从首页重新登录")
				return
			}
			session.Delete("oauth_token")
			token, err := oauthConf.Exchange(context.Background(), call.Code)
			if err != nil {
				c.String(400, "回调验证失败")
				return
			}
			client := github.NewClient(oauthConf.Client(context.Background(), token))
			user, _, err := client.Users.Get(context.Background(), "")
			if err != nil {
				gocd.Log.Errorln(err)
				c.String(500, "GitHub通信失败，请重试")
				return
			}

			// 检测入库
			u, err := userService.UserByGID(user.GetID())
			if err != nil {
				// 首次登陆
				if err == gorm.ErrRecordNotFound {
					u = new(gocd.User)
					u.GID = user.GetID()
					u.GLogin = user.GetLogin()
					u.GName = user.GetName()
					u.GType = user.GetType()
					if userService.CreateUser(u) != nil {
						gocd.Log.Errorln(err)
						c.String(500, "数据库错误")
						return
					}
				} else {
					gocd.Log.Errorln(err)
					c.String(500, "数据库错误")
					return
				}
			} else {
				// 已注册
				u.Token = com.MD5(fmt.Sprintf("%d%d%s%d", u.ID, u.GID, u.GLogin, time.Now().UnixNano()))
				if userService.UpdateUser(u) != nil {
					gocd.Log.Errorln(err)
					c.String(500, "数据库错误")
					return
				} else {
					setCookie(c, "uid", fmt.Sprintf("%d", u.ID))
					setCookie(c, "token", u.Token)
					c.Redirect(http.StatusMovedPermanently, "/")
					return
				}
			}
		})

	}
}
