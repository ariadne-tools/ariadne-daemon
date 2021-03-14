package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"path"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/ariadne-tools/ariadne-daemon/internal/dbconnect"
	"github.com/ariadne-tools/ariadne-daemon/internal/handlergenerator"
	"github.com/ariadne-tools/ariadne-daemon/internal/jsonrpc"
	"github.com/ariadne-tools/ariadne-daemon/internal/logger"
)

const (
	filesdb       = "files.db"
	watcheddirsdb = "watched_dirs.db"
	commitFreq    = 2 * time.Second
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ariadne-daemon",
	Short: "Ariadne-daemon is for create and maintain the indices of chosen directories.",
	Long: `You can use ariadne-daemon to create and maintain a database contains a
complete list of your files and directories of your chosen dir(s).`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		runRoot(runOpts, args)
	},
}

type runOptions struct {
	workDir  string
	logLevel int
}

var runOpts runOptions

func runRoot(options runOptions, args []string) {
	fmt.Println("Welcome to Ariadne daemon!")

	fmt.Println("watchedDirsDB", runOpts.workDir)
	fmt.Println("loglevel", runOpts.logLevel)

	// set dir to the path of the executable
	var dir string
	if ex, err := os.Executable(); err != nil {
		log.Fatal(err)
	} else {
		dir = path.Dir(ex)
	}

	wg := new(sync.WaitGroup)

	filesDbConn := dbconnect.NewDbConnector(path.Join(dir, filesdb), commitFreq, wg)
	watchedDbConn := dbconnect.NewDbConnector(path.Join(dir, watcheddirsdb), 0, wg)
	defer filesDbConn.DB.Close()
	defer watchedDbConn.DB.Close()

	// set all the dirs for full index
	watchedDbConn.Exec("UPDATE watched_dirs SET state_id=?", 1)

	// setting up rpc
	remoteFiles := jsonrpc.RemoteCall{Watcheddb: watchedDbConn, Filesdb: filesDbConn}
	rpc.Register(remoteFiles)
	rpc.HandleHTTP()
	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		io.WriteString(res, "Ariadne's RPC server live!")
	})
	go http.ListenAndServe(":9000", nil)

	go handlergenerator.ProcHandlerGenerator(watchedDbConn, filesDbConn, wg)

	wg.Wait()
	logger.DebugLog("main -> Daemon exiting, bye!")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {

	runFlags := rootCmd.Flags()
	runFlags.StringVar(&runOpts.workDir, "workdir", "~/.config/ariadne-daemon", "set the location of the daemon's working directory")
	runFlags.IntVar(&runOpts.logLevel, "loglevel", 0, "Set the loglevel. 0 means basic level, 1 means debug level")

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	//rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gocobra.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	//rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
