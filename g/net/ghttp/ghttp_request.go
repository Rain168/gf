// Copyright 2017 gf Author(https://gitee.com/johng/gf). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://gitee.com/johng/gf.

package ghttp

import (
    "gitee.com/johng/gf/g/container/gvar"
    "gitee.com/johng/gf/g/encoding/gjson"
    "gitee.com/johng/gf/g/os/gtime"
    "gitee.com/johng/gf/g/util/gregex"
    "gitee.com/johng/gf/third/github.com/fatih/structs"
    "io/ioutil"
    "net/http"
    "strings"
)

// 请求对象
type Request struct {
    http.Request
    parsedGet     bool                // GET参数是否已经解析
    parsedPost    bool                // POST参数是否已经解析
    queryVars     map[string][]string // GET参数
    routerVars    map[string][]string // 路由解析参数
    exit          bool                // 是否退出当前请求流程执行
    Id            int                 // 请求id(唯一)
    Server        *Server             // 请求关联的服务器对象
    Cookie        *Cookie             // 与当前请求绑定的Cookie对象(并发安全)
    Session       *Session            // 与当前请求绑定的Session对象(并发安全)
    Response      *Response           // 对应请求的返回数据操作对象
    Router        *Router             // 匹配到的路由对象
    EnterTime     int64               // 请求进入时间(微秒)
    LeaveTime     int64               // 请求完成时间(微秒)
    Param         interface{}         // 开发者自定义参数
    parsedHost    string              // 解析过后不带端口号的服务器域名名称
    clientIp      string              // 解析过后的客户端IP地址
    isFileRequest bool                // 是否为静态文件请求(非服务请求，当静态文件存在时，优先级会被服务请求高，被识别为文件请求)
    isFileServe   bool                // 是否为文件处理(调用Server.serveFile时设置为true), isFileRequest为true时isFileServe也为true
}

// 创建一个Request对象
func newRequest(s *Server, r *http.Request, w http.ResponseWriter) *Request {
    request := &Request {
        routerVars : make(map[string][]string),
        Id         : s.servedCount.Add(1),
        Server     : s,
        Request    : *r,
        Response   : newResponse(s, w),
        EnterTime  : gtime.Microsecond(),
    }
    // 会话处理
    request.Cookie           = GetCookie(request)
    request.Session          = GetSession(request)
    request.Response.request = request
    return request
}

// 获取Web Socket连接对象(如果是非WS请求会失败，注意检查然会的error结果)
func (r *Request) WebSocket() (*WebSocket, error) {
    if conn, err := wsUpgrader.Upgrade(r.Response.ResponseWriter.ResponseWriter, &r.Request, nil); err == nil {
        return &WebSocket {
            conn,
        }, nil
    } else {
        return nil, err
    }
}

// 获得指定名称的参数字符串(Router/GET/POST)，同 GetRequestString
// 这是常用方法的简化别名
func (r *Request) Get(key string, def ... string) string {
    return r.GetRequestString(key, def...)
}

func (r *Request) GetVar(key string, def ... interface{}) *gvar.Var {
    return r.GetRequestVar(key, def...)
}

// 获取原始请求输入字符串，注意：只能获取一次，读完就没了
func (r *Request) GetRaw() []byte {
    result, _ := ioutil.ReadAll(r.Body)
    return result
}

// 获取原始json请求输入字符串，并解析为json对象
func (r *Request) GetJson() *gjson.Json {
    data := r.GetRaw()
    if data != nil {
        if j, err := gjson.DecodeToJson(data); err == nil {
            return j
        }
    }
    return nil
}

func (r *Request) GetString(key string, def ... string) string {
    return r.GetRequestString(key, def...)
}

func (r *Request) GetInt(key string, def ... int) int {
    return r.GetRequestInt(key, def...)
}

func (r *Request) GetInts(key string, def ... []int) []int {
    return r.GetRequestInts(key, def...)
}

func (r *Request) GetUint(key string, def ... uint) uint {
    return r.GetRequestUint(key, def...)
}

func (r *Request) GetFloat32(key string, def ... float32) float32 {
    return r.GetRequestFloat32(key, def...)
}

func (r *Request) GetFloat64(key string, def ... float64) float64 {
    return r.GetRequestFloat64(key, def...)
}

func (r *Request) GetFloats(key string, def ... []float64) []float64 {
    return r.GetRequestFloats(key, def...)
}

func (r *Request) GetArray(key string, def ... []string) []string {
    return r.GetRequestArray(key, def...)
}

func (r *Request) GetStrings(key string, def ... []string) []string {
    return r.GetRequestStrings(key, def...)
}

func (r *Request) GetInterfaces(key string, def ... []interface{}) []interface{} {
    return r.GetRequestInterfaces(key, def...)
}

func (r *Request) GetMap(def...map[string]string) map[string]string {
    return r.GetRequestMap(def...)
}

// 将所有的request参数映射到struct属性上，参数object应当为一个struct对象的指针, mapping为非必需参数，自定义参数与属性的映射关系
func (r *Request) GetToStruct(object interface{}, mapping...map[string]string) {
    r.GetRequestToStruct(object, mapping...)
}

// 退出当前请求执行，原理是在Request.exit做标记，由服务逻辑流程做判断，自行停止
func (r *Request) Exit() {
    r.exit = true
    panic(gEXCEPTION_EXIT)
}

// 判断当前请求是否停止执行
func (r *Request) IsExited() bool {
    return r.exit
}

// 获取请求的服务端IP/域名
func (r *Request) GetHost() string {
    if len(r.parsedHost) == 0 {
        array, _ := gregex.MatchString(`(.+):(\d+)`, r.Host)
        if len(array) > 1 {
            r.parsedHost = array[1]
        } else {
            r.parsedHost = r.Host
        }
    }
    return r.parsedHost
}

// 判断是否为静态文件请求
func (r *Request) IsFileRequest() bool {
    return r.isFileRequest
}

// 判断请求是否为文件处理
func (r *Request) IsFileServe() bool {
    return r.isFileServe
}

// 判断是否为AJAX请求
func (r *Request) IsAjaxRequest() bool {
    return strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest")
}

// 获取请求的客户端IP地址
func (r *Request) GetClientIp() string {
    if len(r.clientIp) == 0 {
        if r.clientIp = r.Header.Get("X-Real-IP"); r.clientIp == "" {
            array, _ := gregex.MatchString(`(.+):(\d+)`, r.RemoteAddr)
            if len(array) > 1 {
                r.clientIp = array[1]
            } else {
                r.clientIp = r.RemoteAddr
            }
        }
    }
    return r.clientIp
}

// 获得来源URL地址
func (r *Request) GetReferer() string {
    return r.Header.Get("Referer")
}

// 获得结构体顶替的参数名称标签，构成map返回
func (r *Request) getStructParamsTagMap(object interface{}) map[string]string {
    tagmap := make(map[string]string)
    fields := structs.Fields(object)
    for _, field := range fields {
        if tag := field.Tag("params"); tag != "" {
            for _, v := range strings.Split(tag, ",") {
                tagmap[strings.TrimSpace(v)] = field.Name()
            }
        }
    }
    return tagmap
}