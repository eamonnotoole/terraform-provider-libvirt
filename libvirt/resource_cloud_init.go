package libvirt

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/hashicorp/terraform/helper/schema"
	"log"
)

func resourceCloudInit() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudInitCreate,
		Read:   resourceCloudInitRead,
		Delete: resourceCloudInitDelete,
		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"pool": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "default",
				ForceNew: true,
			},
			"local_hostname": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"ssh_authorized_key": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"volid": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "cidata",
				ForceNew: true,
			},
			"user_data_path": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "user-data",
				ForceNew: true,
			},
			"user_data": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "",
				StateFunc: func(v interface{}) string {
					switch v.(type) {
					case string:
						return userDataHashSum(v.(string))
					default:
						return ""
					}
				},
			},
		},
	}
}

func resourceCloudInitCreate(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[DEBUG] creating cloudinit")
	var sshKey string = ""
	virConn := meta.(*Client).libvirt
	if virConn == nil {
		return fmt.Errorf("The libvirt connection was nil.")
	}

	cloudInit := newCloudInitDef()
	cloudInit.Metadata.LocalHostname = d.Get("local_hostname").(string)

	if _, ok := d.GetOk("ssh_authorized_key"); ok {
		sshKey = d.Get("ssh_authorized_key").(string)
		cloudInit.UserData.SSHAuthorizedKeys = append(
			cloudInit.UserData.SSHAuthorizedKeys,
			sshKey)
	}

	cloudInit.Name = d.Get("name").(string)
	cloudInit.PoolName = d.Get("pool").(string)
	cloudInit.UserDataPath = d.Get("user_data_path").(string)
	cloudInit.Volid = d.Get("volid").(string)
	cloudInit.UserDataContent = d.Get("user_data").(string)

	if cloudInit.UserDataContent != "" && sshKey != "" {
		log.Printf("[WARN] Both user_data and ssh_authorized_keys specified, will only use user_data")
	}

	log.Printf("[INFO] cloudInit: %+v", cloudInit)

	key, err := cloudInit.CreateAndUpload(virConn)
	if err != nil {
		return err
	}
	d.SetId(key)

	// make sure we record the id even if the rest of this gets interrupted
	d.Partial(true) // make sure we record the id even if the rest of this gets interrupted
	d.Set("id", key)
	d.SetPartial("id")
	// TODO: at this point we have collected more things than the ID, so let's save as many things as we can
	d.Partial(false)

	return resourceCloudInitRead(d, meta)
}

func resourceCloudInitRead(d *schema.ResourceData, meta interface{}) error {
	virConn := meta.(*Client).libvirt
	if virConn == nil {
		return fmt.Errorf("The libvirt connection was nil.")
	}

	ci, err := newCloudInitDefFromRemoteISO(virConn, d.Id())
	d.Set("pool", ci.PoolName)
	d.Set("name", ci.Name)
	d.Set("local_hostname", ci.Metadata.LocalHostname)

	if err != nil {
		return fmt.Errorf("Error while retrieving remote ISO: %s", err)
	}

	if len(ci.UserData.SSHAuthorizedKeys) == 1 {
		d.Set("ssh_authorized_key", ci.UserData.SSHAuthorizedKeys[0])
	}

	return nil
}

func resourceCloudInitDelete(d *schema.ResourceData, meta interface{}) error {
	virConn := meta.(*Client).libvirt
	if virConn == nil {
		return fmt.Errorf("The libvirt connection was nil.")
	}

	key, err := getCloudInitVolumeKeyFromTerraformID(d.Id())
	if err != nil {
		return err
	}

	return RemoveVolume(virConn, key)
}

func userDataHashSum(user_data string) string {
	// Check whether the user_data is not Base64 encoded.
	// Always calculate hash of base64 decoded value since we
	// check against double-encoding when setting it
	v, base64DecodeError := base64.StdEncoding.DecodeString(user_data)
	if base64DecodeError != nil {
		v = []byte(user_data)
	}

	hash := sha1.Sum(v)
	return hex.EncodeToString(hash[:])
}

func userDataDecode(user_data string) string {
	if user_data != "" {
		v, base64DecodeError := base64.StdEncoding.DecodeString(user_data)
//		log.Printf("[DEBUG] userDataDecode: T%T v %s", v, v)
//		log.Printf("[DEBUG] userDataDecode: base64DecodeError %s", base64DecodeError)
		if base64DecodeError == nil {
//				log.Printf("[DEBUG] userDataDecode: string(v) %s", v)
			return string(v)
		} else {
			return user_data
		}
	}
	return user_data
}
