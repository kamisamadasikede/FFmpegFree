// Package builtinskills contains the redistributable, versioned ffc skill library.
package builtinskills

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/catagent"
)

// All skill resources are shipped with the executable; no network is needed to
// discover or install instructions. Runtime dependencies are installed separately.
//go:embed skills runtime licenses catalog.json
var content embed.FS

type Entry struct {
	Name string `json:"name"`
	Source string `json:"source"`
	Revision string `json:"revision"`
	License string `json:"license"`
	Adapted bool `json:"adapted"`
}
type Catalog struct {Version string `json:"version"`; Skills []Entry `json:"skills"`}
type Report struct {
	Installed []string `json:"installed"`
	Updated []string `json:"updated"`
	Warnings []string `json:"warnings"`
}
type ownership struct {Version string `json:"version"`; Files map[string]map[string]string `json:"files"`}

func List()(Catalog,error){var c Catalog;b,err:=content.ReadFile("catalog.json");if err==nil{err=json.Unmarshal(b,&c)};return c,err}
func hash(b []byte)string{s:=sha256.Sum256(b);return hex.EncodeToString(s[:])}
func tree(source string)(map[string][]byte,error){
	out:=map[string][]byte{}
	err:=fs.WalkDir(content,source,func(path string,d fs.DirEntry,err error)error{
		if err!=nil{return err};if d.IsDir(){return nil}
		b,e:=content.ReadFile(path);if e!=nil{return e};out[strings.TrimPrefix(path,source+"/")]=b;return nil
	});return out,err
}
func fingerprint(files map[string][]byte)map[string]string{out:=map[string]string{};for p,b:=range files{out[p]=hash(b)};return out}
func equal(a,b map[string]string)bool{if len(a)!=len(b){return false};for k,v:=range a{if b[k]!=v{return false}};return true}
func diskTree(root string)(map[string]string,error){
	out:=map[string]string{}
	err:=filepath.WalkDir(root,func(path string,d fs.DirEntry,err error)error{
		if err!=nil{return err};if d.Type()&os.ModeSymlink!=0{return fmt.Errorf("contains a symbolic link")};if d.IsDir(){return nil}
		if !d.Type().IsRegular(){return fmt.Errorf("contains a non-regular file")}
		b,e:=os.ReadFile(path);if e!=nil{return e};rel,e:=filepath.Rel(root,path);if e!=nil{return e};out[filepath.ToSlash(rel)]=hash(b);return nil
	});return out,err
}

// Install creates shared sources only; native CLI links remain owned by the
// existing sharing service. Call with sharingMu held. Local edits and same-name
// custom skills are retained and reported, never silently replaced on upgrade.
func Install(skillsRoot string)(Report,error){
	r:=Report{Installed:[]string{},Updated:[]string{},Warnings:[]string{}}
	if !filepath.IsAbs(skillsRoot){return r,fmt.Errorf("shared skills root must be absolute")}
	if st,e:=os.Lstat(skillsRoot);e==nil&&st.Mode()&os.ModeSymlink!=0{return r,fmt.Errorf("shared skills root must not be a symbolic link")}
	c,err:=List();if err!=nil{return r,err}
	manifestPath:=filepath.Join(skillsRoot,".ffc-builtin.json")
	state:=ownership{Files:map[string]map[string]string{}}
	if b,e:=os.ReadFile(manifestPath);e==nil{if len(b)>4<<20||json.Unmarshal(b,&state)!=nil||state.Files==nil{return r,fmt.Errorf("内置技能记录无法解析，保留现有技能")}}else if !os.IsNotExist(e){return r,e}
	if err=os.MkdirAll(skillsRoot,0700);err!=nil{return r,err}
	sources:=[]string{"runtime","licenses"}
	for _,entry:=range c.Skills{sources=append(sources,"skills/"+entry.Name)}
	for _,source:=range sources{
		name:=strings.TrimPrefix(source,"skills/");destName:=name
		if source=="runtime"{destName=".ffc-runtime"};if source=="licenses"{destName=".ffc-licenses"}
		files,e:=tree(source);if e!=nil{return r,e};desired:=fingerprint(files);dest:=filepath.Join(skillsRoot,destName)
		current,e:=diskTree(dest);exists:=e==nil
		if e!=nil&&!os.IsNotExist(e){r.Warnings=append(r.Warnings,"内置技能目录无法安全更新，已保留："+name);continue}
		previous,owned:=state.Files[destName]
		if exists {
			if !owned{r.Warnings=append(r.Warnings,"共享库已有同名自定义内容，已保留："+name);continue}
			if equal(current,desired){continue}
			if !equal(current,previous){r.Warnings=append(r.Warnings,"内置技能已由用户修改，已保留："+name);continue}
		}
		// Atomically replace a complete directory. Both staging paths are generated
		// children of the absolute shared root, never derived from user skill names.
		temp,e:=os.MkdirTemp(skillsRoot,".ffc-stage-");if e!=nil{return r,e}
		writeErr:=func()error{
			for rel,b:=range files{path:=filepath.Join(temp,filepath.FromSlash(rel));if e:=os.MkdirAll(filepath.Dir(path),0700);e!=nil{return e};if e:=os.WriteFile(path,b,0600);e!=nil{return e}}
			if exists{
				backup,e:=os.MkdirTemp(skillsRoot,".ffc-old-");if e!=nil{return e};if e=os.Remove(backup);e!=nil{return e}
				if e=os.Rename(dest,backup);e!=nil{return e}
				if e=os.Rename(temp,dest);e!=nil{_=os.Rename(backup,dest);return e}
				_=os.RemoveAll(backup)
				return nil
			};return os.Rename(temp,dest)
		}()
		_=os.RemoveAll(temp)
		if writeErr!=nil{return r,writeErr}
		state.Files[destName]=desired
		if strings.HasPrefix(source,"skills/"){if exists{r.Updated=append(r.Updated,name)}else{r.Installed=append(r.Installed,name)}}
	}
	state.Version=c.Version;b,err:=json.MarshalIndent(state,"","  ");if err!=nil{return r,err}
	if err=catagent.WriteShareFile(manifestPath,b);err!=nil{return r,err}
	metadata,_:=content.ReadFile("catalog.json");err=catagent.WriteShareFile(filepath.Join(skillsRoot,".ffc-catalog.json"),metadata)
	return r,err
}
