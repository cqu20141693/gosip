package gb28181

import (
	"time"

	"github.com/cqu20141693/go-service-common/mysql"
	"github.com/cqu20141693/go-service-common/web"
	"github.com/gin-gonic/gin"
)

type CameraDO struct {
	Id        uint64    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `gorm:"column:create_at" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:update_at" json:"updatedAt"`
	GroupKey  string    `gorm:"column:group_key" json:"groupKey"`
	Sn        string
	CameraId  string `gorm:"column:camera_id" json:"cameraId"`
	Token     string
}

func (p CameraDO) TableName() string {
	return "tb_camera"
}

type CameraVO struct {
	GroupKey string `json:"groupKey" binding:"required,max=16,min=16"`
	Sn       string `binding:"required,max=32"`
	CameraId string `json:"cameraId" binding:"required,max=64"`
	Token    string `binding:"required"`
}

type CameraDTO struct {
	*CameraDO
}

type UpdateReq struct {
	GroupKey string `json:"groupKey" binding:"required,max=16,min=16"`
	Sn       string `binding:"required,max=32"`
	CameraId string `json:"cameraId" binding:"required,max=64"`
	Token    string `binding:"required"`
}

//func

func ConvertCameraDTO(do *CameraDO) *CameraDTO {
	return &CameraDTO{
		do,
	}
}

type CameraService struct {
	web.BaseRestController
}

func (c2 *CameraService) InitRouterMapper(router *gin.Engine) {
	router.POST("api/device/camera/insert", c2.Insert)
	router.GET("api/device/camera/:camera_id", c2.FindById)
	router.PATCH("api/device/camera/:camera_id", c2.UpdateById)
}

func (c2 *CameraService) Insert(c *gin.Context) {
	var cameraVo CameraVO
	err := c.ShouldBindJSON(&cameraVo)
	if err != nil {
		logger.Info("binding failed")
		c2.ResponseFailureForParameter(c, err.Error())
		return
	}
	do := CameraDO{
		GroupKey:  cameraVo.GroupKey,
		Sn:        cameraVo.Sn,
		CameraId:  cameraVo.CameraId,
		Token:     cameraVo.Token,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	results := mysql.MysqlDB.Create(&do)
	if results.Error != nil {
		logger.Info("insert failed")
		c2.ResponseFailureForParameter(c, err.Error())
		return
	}
	c2.ResponseData(c, do.Id)
}
func (c2 *CameraService) FindById(c *gin.Context) {
	camera, err := GetByCameraId(c.Param("camera_id"))
	if err != nil {
		c2.ResponseFailureForParameter(c, err.Error())
		return
	}
	c2.ResponseData(c, camera)
}
func (c2 *CameraService) UpdateById(c *gin.Context) {
	var camera = &CameraDO{}
	err := c.BindJSON(camera)
	if err != nil {
		return
	}
	camera, err = UpdateByCameraId(c.Param("camera_id"), camera)
	if err != nil {
		c2.ResponseFailureForParameter(c, err.Error())
		return
	}
	c2.ResponseData(c, camera)
}
func UpdateByCameraId(cameraId string, camera *CameraDO) (*CameraDO, error) {
	if tx := mysql.MysqlDB.Where("camera_id=?", cameraId).Updates(camera); tx.Error != nil {
		return nil, tx.Error
	}
	return camera, nil
}
func GetByCameraId(cameraId string) (*CameraDO, error) {
	var camera = CameraDO{}
	if tx := mysql.MysqlDB.Where("camera_id=?", cameraId).First(&camera); tx.Error != nil {
		return nil, tx.Error
	}
	return &camera, nil
}
