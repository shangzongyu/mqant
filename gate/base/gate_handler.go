// Copyright 2014 mqant Author. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package basegate handler
package basegate

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/shangzongyu/mqant/gate"
	"github.com/shangzongyu/mqant/log"
)

var errorNoSessionFound = errors.New("no session found")

type handler struct {
	// gate.AgentLearner
	// gate.GateHandler
	lock     sync.RWMutex
	gate     gate.Gate
	sessions sync.Map // 连接列表
	agentNum int
}

// NewGateHandler NewGateHandler
func NewGateHandler(gate gate.Gate) *handler {
	handler := &handler{
		gate: gate,
	}
	return handler
}

// Connect 当连接建立  并且MQTT协议握手成功
func (h *handler) Connect(a gate.Agent) {
	defer func() {
		if err := recover(); err != nil {
			buff := make([]byte, 1024)
			runtime.Stack(buff, false)
			log.Error("handler Connect panic(%v)\n info:%s", err, string(buff))
		}
	}()
	if a.GetSession() != nil {
		h.sessions.Store(a.GetSession().GetSessionID(), a)
		// 已经建联成功的才计算
		if a.ProtocolOK() {
			h.lock.Lock()
			h.agentNum++
			h.lock.Unlock()
		}
	}
	if h.gate.GetSessionLearner() != nil {
		go func() {
			h.gate.GetSessionLearner().Connect(a.GetSession())
		}()
	}
}

// DisConnect 当连接关闭	或者客户端主动发送MQTT DisConnect命令
func (h *handler) DisConnect(a gate.Agent) {
	defer func() {
		if err := recover(); err != nil {
			buff := make([]byte, 1024)
			runtime.Stack(buff, false)
			log.Error("handler DisConnect panic(%v)\n info:%s", err, string(buff))
		}
		if a.GetSession() != nil {
			h.sessions.Delete(a.GetSession().GetSessionID())
			// 已经建联成功的才计算
			if a.ProtocolOK() {
				h.lock.Lock()
				h.agentNum--
				h.lock.Unlock()
			}
		}
	}()
	if h.gate.GetSessionLearner() != nil {
		if a.GetSession() != nil {
			// 没有session的就不返回了
			h.gate.GetSessionLearner().DisConnect(a.GetSession())
		}
	}
}

func (h *handler) OnDestroy() {
	h.sessions.Range(func(key, value interface{}) bool {
		value.(gate.Agent).Close()
		h.sessions.Delete(key)
		return true
	})
}

func (h *handler) GetAgentNum() int {
	num := 0
	h.lock.RLock()
	num = h.agentNum
	h.lock.RUnlock()
	return num
}

// GetAgent 更新整个Session 通常是其他模块拉取最新数据
func (h *handler) GetAgent(SessionId string) (gate.Agent, error) {
	agentById, ok := h.sessions.Load(SessionId)
	if !ok || agentById == nil {
		return nil, errorNoSessionFound
	}
	return agentById.(gate.Agent), nil
}

// Update 更新整个Session 通常是其他模块拉取最新数据
func (h *handler) Update(span log.TraceSpan, SessionId string) (result gate.Session, err string) {
	agentById, ok := h.sessions.Load(SessionId)
	if !ok || agentById == nil {
		err = errorNoSessionFound.Error()
		return
	}
	result = agentById.(gate.Agent).GetSession()
	return
}

// Bind the session with  the Userid.
func (h *handler) Bind(span log.TraceSpan, SessionId string, Userid string) (result gate.Session, err string) {
	agentById, ok := h.sessions.Load(SessionId)
	if !ok || agentById == nil {
		err = errorNoSessionFound.Error()
		return
	}
	agentById.(gate.Agent).GetSession().SetUserID(Userid)

	if h.gate.GetStorageHandler() != nil && agentById.(gate.Agent).GetSession().GetUserID() != "" {
		// 可以持久化
		data, err := h.gate.GetStorageHandler().Query(Userid)
		if err == nil && data != nil {
			// 有已持久化的数据,可能是上一次连接保存的
			impSession, err := h.gate.NewSession(data)
			if err == nil {
				if agentById.(gate.Agent).GetSession() == nil {
					agentById.(gate.Agent).GetSession().SetSettings(impSession.CloneSettings())
				} else {
					// 合并两个map 并且以 agent.(Agent).GetSession().Settings 已有的优先
					settings := impSession.CloneSettings()
					_ = agentById.(gate.Agent).GetSession().ImportSettings(settings)
				}
			} else {
				// 解析持久化数据失败
				log.Warning("sesssion Resolve fail %s", err.Error())
			}
		}
		// 数据持久化
		_ = h.gate.GetStorageHandler().Storage(agentById.(gate.Agent).GetSession())
	}

	result = agentById.(gate.Agent).GetSession()
	return
}

// IsConnect 查询某一个userId是否连接中，这里只是查询这一个网关里面是否有userId客户端连接，如果有多个网关就需要遍历了
func (h *handler) IsConnect(span log.TraceSpan, sessionId string, Userid string) (bool, string) {
	isConnect := false
	found := false
	h.sessions.Range(func(key, agent interface{}) bool {
		if agent.(gate.Agent).GetSession().GetUserID() == Userid {
			isConnect = !agent.(gate.Agent).IsClosed()
			found = true
			return false
		}
		return true
	})
	if !found {
		return false, fmt.Sprintf("The gateway did not find the corresponding userId 【%s】", Userid)
	}

	return isConnect, ""
}

// UnBind the session with the Userid.
func (h *handler) UnBind(span log.TraceSpan, sessionId string) (result gate.Session, err string) {
	agentById, ok := h.sessions.Load(sessionId)
	if !ok || agentById == nil {
		err = errorNoSessionFound.Error()
		return
	}
	agentById.(gate.Agent).GetSession().SetUserID("")
	result = agentById.(gate.Agent).GetSession()
	return
}

// Push the session with the Userid.
func (h *handler) Push(span log.TraceSpan, SessionId string, Settings map[string]string) (result gate.Session, err string) {
	agentById, ok := h.sessions.Load(SessionId)
	if !ok || agentById == nil {
		err = errorNoSessionFound.Error()
		return
	}
	// 覆盖当前map对应的key-value
	for key, value := range Settings {
		_ = agentById.(gate.Agent).GetSession().SetLocalKV(key, value)
	}
	result = agentById.(gate.Agent).GetSession()
	if h.gate.GetStorageHandler() != nil && agentById.(gate.Agent).GetSession().GetUserID() != "" {
		err := h.gate.GetStorageHandler().Storage(agentById.(gate.Agent).GetSession())
		if err != nil {
			log.Warning("gate session storage failure : %s", err.Error())
		}
	}

	return
}

// Set values (one or many) for the session.
func (h *handler) Set(span log.TraceSpan, sessionId, key, value string) (result gate.Session, err string) {
	agentById, ok := h.sessions.Load(sessionId)
	if !ok || agentById == nil {
		err = errorNoSessionFound.Error()
		return
	}
	_ = agentById.(gate.Agent).GetSession().SetLocalKV(key, value)
	result = agentById.(gate.Agent).GetSession()

	if h.gate.GetStorageHandler() != nil && agentById.(gate.Agent).GetSession().GetUserID() != "" {
		err := h.gate.GetStorageHandler().Storage(agentById.(gate.Agent).GetSession())
		if err != nil {
			log.Error("gate session storage failure : %s", err.Error())
		}
	}

	return
}

// Remove value from the session.
func (h *handler) Remove(span log.TraceSpan, SessionId string, key string) (result interface{}, err string) {
	agentById, ok := h.sessions.Load(SessionId)
	if !ok || agentById == nil {
		err = errorNoSessionFound.Error()
		return
	}
	_ = agentById.(gate.Agent).GetSession().RemoveLocalKV(key)
	result = agentById.(gate.Agent).GetSession()

	if h.gate.GetStorageHandler() != nil && agentById.(gate.Agent).GetSession().GetUserID() != "" {
		err := h.gate.GetStorageHandler().Storage(agentById.(gate.Agent).GetSession())
		if err != nil {
			log.Error("gate session storage failure :%s", err.Error())
		}
	}

	return
}

// Send message to the session.
func (h *handler) Send(span log.TraceSpan, SessionId string, topic string, body []byte) (result interface{}, err string) {
	agentById, ok := h.sessions.Load(SessionId)
	if !ok || agentById == nil {
		err = errorNoSessionFound.Error()
		return
	}
	e := agentById.(gate.Agent).WriteMsg(topic, body)
	if e != nil {
		err = e.Error()
	} else {
		result = "success"
	}
	return
}

// SendBatch 批量发送消息,sessionIdStr 之间用,分割
func (h *handler) SendBatch(span log.TraceSpan, sessionIdStr string, topic string, body []byte) (int64, string) {
	sessionIds := strings.Split(sessionIdStr, ",")
	var count int64 = 0
	for _, sessionId := range sessionIds {
		agentById, ok := h.sessions.Load(sessionId)
		if !ok || agentById == nil {
			continue
		}
		e := agentById.(gate.Agent).WriteMsg(topic, body)
		if e != nil {
			log.Warning("WriteMsg error: %v", e.Error())
		} else {
			count++
		}
	}
	return count, ""
}

func (h *handler) BroadCast(span log.TraceSpan, topic string, body []byte) (int64, string) {
	var count int64 = 0
	h.sessions.Range(func(key, agent interface{}) bool {
		e := agent.(gate.Agent).WriteMsg(topic, body)
		if e != nil {
			log.Warning("WriteMsg error:", e.Error())
		} else {
			count++
		}
		return true
	})
	return count, ""
}

// Close 主动关闭连接
func (h *handler) Close(span log.TraceSpan, sessionId string) (result interface{}, err string) {
	agentById, ok := h.sessions.Load(sessionId)
	if !ok || agentById == nil {
		err = errorNoSessionFound.Error()
		return
	}
	agentById.(gate.Agent).Close()
	return
}
