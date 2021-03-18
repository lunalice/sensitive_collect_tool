package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql" //mysql driver
)

func main() {
		now := time.Now()
		exec_cmd(now.Format("2006-01-02"), []string{"cat", "/etc/passwd"}, "passwd")
		exec_cmd(now.Format("2006-01-02"), []string{"cat", "/etc/group"}, "group")
		exec_mysql(now.Format("2006-01-02"), "select Host, User from mysql.user;", "mysql")
		create_mail_template(now)
}

func readFile(filepath string) string {
		b, err := ioutil.ReadFile(filepath)
		if err != nil {
				fmt.Println(os.Stderr, err)
				os.Exit(1)
		}
		data := string(b)

		return data
}

func create_mail_template(now time.Time) {
		location := time.FixedZone("Asia/Tokyo", 9*60*60)
		now = now.In(location)
		merge := now.String() + "\n"
		merge = merge + mergeFiles("passwd", "cat /etc/passwd")
		merge = merge + mergeFiles("group", "cat /etc/group")
		merge = merge + mergeFiles("mysql", "select Host, User from mysql.user;")
		slice := strings.Split(merge, "\n")
		lines := []string{}
		for _, str := range slice {
			if !strings.HasPrefix(str, "#") {
				lines = append(lines, str + "\n")
			}
		}

		b := []byte{}
		for _, line := range lines {
				if line == "\n" {
					continue
				}
				ll := []byte(line)
				for _, l := range ll {
						b = append(b, l)
				}
		}

		err := ioutil.WriteFile("output_mail.txt", b, 0666)
		if err != nil {
				fmt.Println(os.Stderr, err)
				os.Exit(1)
		}
}

func mergeFiles(cmd_type string, cmd string) string {
		// base読み込み
		base := readFile("./template/base_template.txt")
		filepaths := dirwalk("./tmp")
		merge_content := ""
		for _, path := range filepaths {
			if strings.Index(path, cmd_type) != -1 {
				content := readFile("./template/content_template.txt")
				target := readFile(path)

				filename := filepath.Base(path)
				split_name := strings.Split(filename, "_")
				content = strings.Replace(content, "server", split_name[0], -1)
				content = strings.Replace(content, "user", target, -1)
				merge_content = merge_content + content
			}
		}
		base = strings.Replace(base, "command", cmd, -1)
		base = strings.Replace(base, "content", merge_content, -1)
		return base
}

// 再起ファイル取得
func dirwalk(dir string) []string {
    files, err := ioutil.ReadDir(dir)
    if err != nil {
        panic(err)
    }

    var paths []string
    for _, file := range files {
			if file.IsDir() {
				paths = append(paths, dirwalk(filepath.Join(dir, file.Name()))...)
				continue
			}
			paths = append(paths, filepath.Join(dir, file.Name()))
    }

    return paths
}

func exec_mysql(now string, cmd string, cmd_type string) {
	db, err := sql.Open("mysql", dsn())
	if err != nil {
		log.Printf("Mysql server seems down: %s\n", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query("select Host, User from mysql.user;")
	if err != nil {
			log.Fatal(err)
	}
	defer rows.Close()
	b := []byte{}
	ll := []byte("Host User\n")
	for _, l := range ll {
		b = append(b, l)
	}

	for rows.Next() {
		var user string
		var host string
		if err := rows.Scan(&host, &user); err != nil {
			log.Fatal(err)
		}

		ll := []byte(host + " " + user + "\n")
		for _, l := range ll {
			b = append(b, l)
		}
	}

	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	hostname, err1 := exec.Command("hostname").Output()
	if err1 != nil {
		fmt.Println(err1.Error())
		os.Exit(1)
	}

	err = ioutil.WriteFile("./tmp/" + strings.TrimSpace(string(hostname)) + "_" + cmd_type + "_" + now + ".txt", b, 0666)
	if err != nil {
			fmt.Println(os.Stderr, err)
			os.Exit(1)
	}

	fmt.Println(cmd_type + " complete!")
}

func exec_cmd(now string, cmd []string, cmd_type string) {
	out, err := exec.Command(cmd[0], cmd[1]).Output()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	slice := strings.Split(string(out), "\n")
	lines := []string{}
	for _, str := range slice {
		if !strings.HasPrefix(str, "#") {
			lines = append(lines, str + "\n")
		}
	}

	b := []byte{}
	for _, line := range lines {
			if line == "\n" {
				continue
			}
			ll := []byte(line)
			for _, l := range ll {
					b = append(b, l)
			}
	}

	hostname, err1 := exec.Command("hostname").Output()
	if err1 != nil {
		fmt.Println(err1.Error())
		os.Exit(1)
	}

	err = ioutil.WriteFile("./tmp/" + strings.TrimSpace(string(hostname)) + "_" + cmd_type + "_" + now + ".txt", b, 0666)
	if err != nil {
			fmt.Println(os.Stderr, err)
			os.Exit(1)
	}

	fmt.Println(cmd_type + " complete!")
}

type Settings struct {
	DatabaseUser     string `json:"DATABASE_USER"`
	DatabaseHost     string `json:"DATABASE_HOST"`
	DatabasePost     string `json:"DATABASE_PORT"`
	DatabaseName     string `json:"DATABASE_NAME"`
	DatabasePassword string `json:"DATABASE_PASSWORD"`
}

const (
	DefaultExpires = 86400
	ExitCodeError  = 111
	Version        = "0.0.1"
	RetryInterval  = time.Duration(500) * time.Millisecond
)

func dsn() string {
	setting := load_json()
	// "user:password@tcp(127.0.0.1:3306)/dbname?charset=utf8&parseTime=true&loc=Asia%2FTokyo"
	return setting.DatabaseUser + ":" + setting.DatabasePassword + "@tcp(" + setting.DatabaseHost + ":" + setting.DatabasePost + ")/" + setting.DatabaseName // + "?charset=utf8&parseTime=true&loc=Asia%2FTokyo"
}

func load_json() Settings {
	raw, err := ioutil.ReadFile("./settings.json")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	var st Settings

	json.Unmarshal(raw, &st)

	return st
}
