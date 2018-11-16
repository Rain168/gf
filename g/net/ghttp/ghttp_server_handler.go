// Copyright 2017 gf Author(https://gitee.com/johng/gf). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://gitee.com/johng/gf.
// 请求处理.

package ghttp

import (
    "fmt"
    "gitee.com/johng/gf/g/encoding/ghtml"
    "gitee.com/johng/gf/g/os/gfile"
    "gitee.com/johng/gf/g/os/gtime"
    "net/http"
    "net/url"
    "os"
    "reflect"
    "sort"
    "strings"
)

// 默认HTTP Server处理入口，http包底层默认使用了gorutine异步处理请求，所以这里不再异步执行
func (s *Server)defaultHttpHandle(w http.ResponseWriter, r *http.Request) {
    s.handleRequest(w, r)
}

// 执行处理HTTP请求
// 首先，查找是否有对应域名的处理接口配置；
// 其次，如果没有对应的自定义处理接口配置，那么走默认的域名处理接口配置；
// 最后，如果以上都没有找到处理接口，那么进行文件处理；
func (s *Server)handleRequest(w http.ResponseWriter, r *http.Request) {
    // 去掉末尾的"/"号
    if r.URL.Path != "/" {
        r.URL.Path = strings.TrimRight(r.URL.Path, "/")
    }

    // 创建请求处理对象
    request := newRequest(s, r, w)

    defer func() {
        if request.LeaveTime == 0 {
            request.LeaveTime = gtime.Microsecond()
        }
        s.callHookHandler(HOOK_BEFORE_CLOSE, request)
        // access log
        s.handleAccessLog(request)
        // error log使用recover进行判断
        if e := recover(); e != nil {
            s.handleErrorLog(e, request)
        }
        // 更新Session会话超时时间
        request.Session.UpdateExpire()
        s.callHookHandler(HOOK_AFTER_CLOSE, request)
    }()

    // ============================================================
    // 优先级控制:
    // 静态文件 > 动态服务 > 静态目录
    // ============================================================

    // 优先执行静态文件检索(检测是否存在对应的静态文件，包括index files处理)
    staticFile, isStaticDir := s.paths.Search(r.URL.Path, s.config.IndexFiles...)
    if staticFile != "" {
        request.isFileRequest = true
    }

    // 动态服务检索
    handler := (*handlerItem)(nil)
    if !request.IsFileRequest() || isStaticDir {
        if parsedItem := s.getServeHandlerWithCache(request); parsedItem != nil {
            handler = parsedItem.handler
            for k, v := range parsedItem.values {
                request.routerVars[k] = v
            }
            request.Router = parsedItem.handler.router
        }
    }

    // 事件 - BeforeServe
    s.callHookHandler(HOOK_BEFORE_SERVE, request)

    // 执行静态文件服务/回调控制器/执行对象/方法
    if !request.IsExited() {
        // 需要再次判断文件是否真实存在，因为文件检索可能使用了缓存，从健壮性考虑这里需要二次判断
        if request.IsFileRequest() && !isStaticDir /* && gfile.Exists(staticFile) */ && gfile.SelfPath() != staticFile {
            // 静态文件
            s.serveFile(request, staticFile)
        } else {
            if handler != nil {
                // 动态服务
                s.callServeHandler(handler, request)
            } else {
                if isStaticDir {
                    // 静态目录
                    s.serveFile(request, staticFile)
                } else {
                    request.Response.WriteStatus(http.StatusNotFound)
                }
            }
        }
    }

    // 事件 - AfterServe
    s.callHookHandler(HOOK_AFTER_SERVE, request)

    // 设置请求完成时间
    request.LeaveTime = gtime.Microsecond()

    // 事件 - BeforeOutput
    s.callHookHandler(HOOK_BEFORE_OUTPUT, request)
    // 输出Cookie
    request.Cookie.Output()
    // 输出缓冲区
    request.Response.OutputBuffer()
    // 事件 - AfterOutput
    s.callHookHandler(HOOK_AFTER_OUTPUT, request)
}

// 初始化控制器
func (s *Server) callServeHandler(h *handlerItem, r *Request) {
    defer func() {
        if e := recover(); e != nil && e != gEXCEPTION_EXIT {
            panic(e)
        }
    }()
    if h.faddr == nil {
        // 新建一个控制器对象处理请求
        c := reflect.New(h.ctype)
        c.MethodByName("Init").Call([]reflect.Value{reflect.ValueOf(r)})
        if !r.IsExited() {
            c.MethodByName(h.fname).Call(nil)
            c.MethodByName("Shut").Call([]reflect.Value{reflect.ValueOf(r)})
        }
    } else {
        // 是否有初始化及完成回调方法
        if h.finit != nil {
            h.finit(r)
        }
        if !r.IsExited() {
            h.faddr(r)
            if h.fshut != nil {
                h.fshut(r)
            }
        }
    }
}

// http server静态文件处理，path可以为相对路径也可以为绝对路径
func (s *Server)serveFile(r *Request, path string) {
    f, err := os.Open(path)
    if err != nil {
        r.Response.WriteStatus(http.StatusForbidden)
        return
    }
    defer f.Close()
    info, _ := f.Stat()
    if info.IsDir() {
        if s.config.IndexFolder {
            s.listDir(r, f)
        } else {
            r.Response.WriteStatus(http.StatusForbidden)
        }
    } else {
        // 读取文件内容返回, no buffer
        http.ServeContent(r.Response.Writer, &r.Request, info.Name(), info.ModTime(), f)
    }
}

// 目录列表
func (s *Server)listDir(r *Request, f http.File) {
    dirs, err := f.Readdir(-1)
    if err != nil {
        r.Response.WriteStatus(http.StatusInternalServerError, "Error reading directory")
        return
    }
    sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })

    r.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
    r.Response.Write("<pre>\n")
    for _, d := range dirs {
        name := d.Name()
        if d.IsDir() {
            name += "/"
        }
        u := url.URL{Path: name}
        r.Response.Write(fmt.Sprintf("<a href=\"%s\">%s</a>\n", u.String(), ghtml.SpecialChars(name)))
    }
    r.Response.Write("</pre>\n")
}
