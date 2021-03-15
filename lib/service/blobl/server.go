package blobl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/Jeffail/benthos/v3/internal/bloblang"
	"github.com/Jeffail/benthos/v3/internal/bloblang/parser"
	"github.com/urfave/cli/v2"
)

// TODO: When we upgrade to Go 1.16 we can use the new embed stuff.
const bloblangEditorPage = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Bloblang Editor</title>
    <style>
      html, body {
        background-color: #202020;
        margin: 0;
        padding: 0;
        height: 100%;
        width: 100%;
      }
      .panel {
        position: absolute;
        margin: 0;
      }
      .panel > h2 {
        position: absolute;
        text-align: center;
        width: 100px;
        background-color: #33352e;
        color: white;
        font-family: monospace;
        border-bottom: solid #a6e22e 2px;
      }
      #input, #output, #mapping {
        background-color: #33352e;
        height: 100%;
        width: 100%;
        overflow: auto;
        box-sizing: border-box;
        margin: 0;
        padding: 10px;
        font-size: 12pt;
        font-family: monospace;
        color: #fff;
        border: solid #33352e 2px;
      }
      textarea {
        resize: none;
      }
    </style>
  </head>
  <body>
    <div class="panel" style="top:0;bottom:50%;left:0;right:50%;padding:0 5px 5px 0">
      <h2 style="left:50%;bottom:0;margin-left:-50px;">Input</h2>
      <textarea id="input">{"input":"document"}</textarea>
    </div>
    <div class="panel" style="top:0;bottom:50%;left:50%;right:0;padding:0 0 5px 5px">
      <h2 style="left:50%;bottom:0;margin-left:-50px;">Output</h2>
      <pre id="output"></pre>
    </div>
    <div class="panel" style="top:50%;bottom:0;left:0;right:0;padding: 5px 0 0 0">
      <h2 style="left:50%;bottom:0;margin-left:-50px;">Mapping</h2>
      <textarea id="mapping">root = this</textarea>
    </div>
  </body>
  <script>
    function execute() {
        const request = new Request(window.location.href + 'execute', {
            method: 'POST',
            body: JSON.stringify({
                mapping: mappingArea.value,
                input: inputArea.value,
            }),
        });
        fetch(request)
            .then(response => {
                if (response.status === 200) {
                    return response.json();
                } else {
                    throw new Error('Something went wrong on api server!');
                }
            })
            .then(response => {
                const red = "#f92672";
                let result = "No result";
                inputArea.style.borderColor = "#33352e";
                mappingArea.style.borderColor = "#33352e";
                outputArea.style.color = "white";
                if (response.result.length > 0) {
                    result = document.createTextNode(response.result);
                } else if (response.mapping_error.length > 0) {
                    inputArea.style.borderColor = red;
                    outputArea.style.color = red;
                    result = document.createTextNode(response.mapping_error);
                } else if (response.parse_error.length > 0) {
                    mappingArea.style.borderColor = red;
                    outputArea.style.color = red;
                    result = document.createTextNode(response.parse_error);
                }
                outputArea.innerHTML = "";
                outputArea.appendChild(result);
            }).catch(error => {
                console.error(error);
            });
    }

    const mappingArea = document.getElementById("mapping");
    const inputArea = document.getElementById("input");
    const outputArea = document.getElementById("output");
    const inputs = document.getElementsByTagName('textarea');
    for (let input of inputs) {
        input.addEventListener('keydown', function(e) {
            if (e.key == 'Tab') {
                e.preventDefault();
                var start = this.selectionStart;
                var end = this.selectionEnd;

                // set textarea value to: text before caret + tab + text after caret
                this.value = this.value.substring(0, start) +
                    "\t" + this.value.substring(end);

                // put caret at right position again
                this.selectionStart =
                this.selectionEnd = start + 1;
            }
        });
        input.addEventListener('input', function(e) {
            execute();
        })
    }
    execute();
  </script>
</html>`

func openBrowserAt(url string) {
	switch runtime.GOOS {
	case "linux":
		_ = exec.Command("xdg-open", url).Start()
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	}
}

func runServer(c *cli.Context) error {
	http.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		req := struct {
			Mapping string `json:"mapping"`
			Input   string `json:"input"`
		}{}
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		res := struct {
			ParseError   string `json:"parse_error"`
			MappingError string `json:"mapping_error"`
			Result       string `json:"result"`
		}{}
		defer func() {
			resBytes, err := json.Marshal(res)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			w.Write(resBytes)
		}()

		exec, err := bloblang.NewMapping("", req.Mapping)
		if err != nil {
			if perr, ok := err.(*parser.Error); ok {
				res.ParseError = fmt.Sprintf("failed to parse mapping: %v\n", perr.ErrorAtPositionStructured("", []rune(req.Mapping)))
			} else {
				res.ParseError = err.Error()
			}
			return
		}

		output, err := executeMapping(exec, false, true, []byte(req.Input))
		if err != nil {
			res.MappingError = err.Error()
		} else {
			res.Result = output
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(bloblangEditorPage))
	})

	host, port := c.String("host"), c.String("port")
	bindAddress := host + ":" + port

	if !c.Bool("no-open") {
		u, err := url.Parse("http://localhost:" + port)
		if err != nil {
			panic(err)
		}
		openBrowserAt(u.String())
	}

	fmt.Printf("Serving at: http://%v\n", bindAddress)
	return http.ListenAndServe(bindAddress, nil)
}
