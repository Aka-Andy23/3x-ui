package controller

import (
	"errors"
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

type DirectDomainController struct {
	clientService service.ClientService
}

func NewDirectDomainController(g *gin.RouterGroup) *DirectDomainController {
	a := &DirectDomainController{}
	g.GET("/list", a.list)
	g.POST("/upsert", a.upsert)
	g.POST("/import", a.importDomains)
	g.POST("/replace", a.replaceClientDomains)
	g.POST("/del/:id", a.delete)
	return a
}

type directDomainScope struct {
	ClientEmail string `json:"clientEmail" form:"clientEmail"`
}

type directDomainUpsertBody struct {
	ClientEmail string                    `json:"clientEmail"`
	Domain      service.DirectDomainInput `json:"domain"`
}

type directDomainImportBody struct {
	ClientEmail string `json:"clientEmail"`
	Raw         string `json:"raw"`
	Mode        string `json:"mode"`
	Comment     string `json:"comment"`
}

type directDomainReplaceBody struct {
	ClientEmail string `json:"clientEmail"`
	Includes    string `json:"includes"`
	Excludes    string `json:"excludes"`
}

func (a *DirectDomainController) resolveClientId(email string) (int, error) {
	if email == "" {
		return 0, nil
	}
	rec, err := a.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		return 0, err
	}
	return rec.Id, nil
}

func (a *DirectDomainController) list(c *gin.Context) {
	clientId, err := a.resolveClientId(c.Query("clientEmail"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "get"), err)
		return
	}
	rows, err := a.clientService.ListDirectDomains(clientId, c.Query("includeGlobal") == "1")
	if err != nil {
		jsonMsg(c, I18nWeb(c, "get"), err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *DirectDomainController) upsert(c *gin.Context) {
	var body directDomainUpsertBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	clientId, err := a.resolveClientId(body.ClientEmail)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	row, err := a.clientService.UpsertDirectDomain(clientId, body.Domain)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "success"), row, nil)
}

func (a *DirectDomainController) importDomains(c *gin.Context) {
	var body directDomainImportBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	clientId, err := a.resolveClientId(body.ClientEmail)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	result, err := a.clientService.ImportDirectDomains(clientId, body.Raw, body.Mode, body.Comment)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "success"), result, nil)
}

func (a *DirectDomainController) replaceClientDomains(c *gin.Context) {
	var body directDomainReplaceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	clientId, err := a.resolveClientId(body.ClientEmail)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if clientId == 0 {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("client email is required"))
		return
	}
	result, err := a.clientService.ReplaceClientDirectDomains(clientId, body.Includes, body.Excludes)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "success"), result, nil)
}

func (a *DirectDomainController) delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	clientId, err := a.resolveClientId(c.Query("clientEmail"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "success"), a.clientService.DeleteDirectDomain(clientId, id))
}
