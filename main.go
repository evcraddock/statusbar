package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/user"
	"path/filepath"
	"time"

	"barista.run"
	"barista.run/bar"
	"barista.run/base/click"
	"barista.run/base/watchers/netlink"
	"barista.run/colors"
	"barista.run/format"
	"barista.run/group/modal"
	"barista.run/modules/clock"
	"barista.run/modules/cputemp"
	"barista.run/modules/media"
	"barista.run/modules/meminfo"
	"barista.run/modules/meta/split"
	"barista.run/modules/netinfo"
	"barista.run/modules/netspeed"
	"barista.run/modules/volume"
	"barista.run/modules/weather"
	"barista.run/modules/weather/openweathermap"
	"barista.run/modules/wlan"
	"barista.run/outputs"
	"barista.run/pango"
	"barista.run/pango/icons/fontawesome"
	"barista.run/pango/icons/ionicons"
	"barista.run/pango/icons/material"
	"barista.run/pango/icons/mdi"
	"barista.run/pango/icons/typicons"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/martinlindhe/unit"
)

var spacer = pango.Text(" ").XXSmall()
var mainModalController modal.Controller

func makeIconOutput(key string) *bar.Segment {
	return outputs.Pango(spacer, pango.Icon(key), spacer)
}

func mediaFormatFunc(m media.Info) bar.Output {
	if m.PlaybackStatus == media.Stopped || m.PlaybackStatus == media.Disconnected {
		return nil
	}
	artist := truncate(m.Artist, 35)
	title := truncate(m.Title, 70-len(artist))
	if len(title) < 35 {
		artist = truncate(m.Artist, 35-len(title))
	}
	var iconAndPosition bar.Output
	if m.PlaybackStatus == media.Playing {
		iconAndPosition = outputs.Repeat(func(time.Time) bar.Output {
			return makeMediaIconAndPosition(m)
		}).Every(time.Second)
	} else {
		iconAndPosition = makeMediaIconAndPosition(m)
	}
	return outputs.Group(iconAndPosition, outputs.Pango(title, " - ", artist))
}

func makeMediaIconAndPosition(m media.Info) *pango.Node {
	iconAndPosition := pango.Icon("fa-music").Color(colors.Hex("#f70"))
	if m.PlaybackStatus == media.Playing {
		iconAndPosition.Append(spacer,
			pango.Textf("%s/", formatMediaTime(m.Position())))
	}
	if m.PlaybackStatus == media.Paused || m.PlaybackStatus == media.Playing {
		iconAndPosition.Append(spacer,
			pango.Textf("%s", formatMediaTime(m.Length)))
	}
	return iconAndPosition
}

func formatMediaTime(d time.Duration) string {
	h, m, s := hms(d)
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func hms(d time.Duration) (h int, m int, s int) {
	h = int(d.Hours())
	m = int(d.Minutes()) % 60
	s = int(d.Seconds()) % 60
	return
}

func truncate(in string, l int) string {
	fromStart := false
	if l < 0 {
		fromStart = true
		l = -l
	}
	inLen := len([]rune(in))
	if inLen <= l {
		return in
	}
	if fromStart {
		return "⋯" + string([]rune(in)[inLen-l+1:])
	}
	return string([]rune(in)[:l-1]) + "⋯"
}

func main() {
	material.Load(home("code/src/github.com/material-design-icons"))
	mdi.Load(home("code/src/github.com/MaterialDesign-Webfont"))
	typicons.Load(home("code/src/github.com/typicons.font"))
	ionicons.LoadMd(home("code/src/github.com/ionicons"))
	fontawesome.Load(home("code/src/github.com/Font-Awesome"))

	colors.LoadBarConfig()
	bg := colors.Scheme("background")
	fg := colors.Scheme("statusline")
	if fg != nil && bg != nil {
		iconColor := fg.Colorful().BlendHcl(bg.Colorful(), 0.5).Clamped()
		colors.Set("dim-icon", iconColor)
		_, _, v := fg.Colorful().Hsv()
		if v < 0.3 {
			v = 0.3
		}
		colors.Set("bad", colorful.Hcl(40, 1.0, v).Clamped())
		colors.Set("degraded", colorful.Hcl(90, 1.0, v).Clamped())
		colors.Set("good", colorful.Hcl(120, 1.0, v).Clamped())
	}

	vol := volume.DefaultMixer().Output(func(v volume.Volume) bar.Output {
		if v.Mute {
			return outputs.
				Pango(pango.Icon("ion-volume-off").Alpha(0.8), spacer, "MUT").
				Color(colors.Scheme("degraded"))
		}
		iconName := "mute"
		pct := v.Pct()
		if pct > 66 {
			iconName = "high"
		} else if pct > 33 {
			iconName = "low"
		}
		return outputs.Pango(
			pango.Icon("ion-volume-"+iconName).Alpha(0.6),
			spacer,
			pango.Textf("%2d%%", pct),
		)
	})

	mediaSummary, mediaDetail := split.New(media.Auto().Output(mediaFormatFunc), 1)

	freeMem := meminfo.New().Output(func(m meminfo.Info) bar.Output {
		out := outputs.Pango(pango.Icon("material-memory"), format.IBytesize(m.Available()))
		freeGigs := m.Available().Gigabytes()
		switch {
		case freeGigs < 0.5:
			out.Urgent(true)
		case freeGigs < 1:
			out.Color(colors.Scheme("bad"))
		case freeGigs < 2:
			out.Color(colors.Scheme("degraded"))
		case freeGigs > 12:
			out.Color(colors.Scheme("good"))
		}
		out.OnClick(startTaskManager)
		return out
	})

	temp := cputemp.New().
		RefreshInterval(2 * time.Second).
		Output(func(temp unit.Temperature) bar.Output {
			out := outputs.Pango(
				pango.Icon("mdi-fan"), spacer,
				pango.Textf("%2d°F", int(temp.Fahrenheit())),
			)
			switch {
			case temp.Celsius() > 90:
				out.Urgent(true)
			case temp.Celsius() > 70:
				out.Color(colors.Scheme("bad"))
			case temp.Celsius() > 60:
				out.Color(colors.Scheme("degraded"))
			}
			return out
		})

	sub := netlink.Any()
	iface := sub.Get().Name
	sub.Unsubscribe()
	netsp := netspeed.New(iface).
		RefreshInterval(2 * time.Second).
		Output(func(s netspeed.Speeds) bar.Output {
			return outputs.Pango(
				pango.Icon("fa-upload").Alpha(0.5), spacer, pango.Textf("%7s", format.Byterate(s.Tx)),
				pango.Text(" ").Small(),
				pango.Icon("fa-download").Alpha(0.5), spacer, pango.Textf("%7s", format.Byterate(s.Rx)),
			)
		})

	net := netinfo.New().Output(func(i netinfo.State) bar.Output {
		if !i.Enabled() {
			return nil
		}
		if i.Connecting() || len(i.IPs) < 1 {
			return outputs.Text(i.Name).Color(colors.Scheme("degraded"))
		}
		return outputs.Group(outputs.Text(i.Name), outputs.Textf("%s", i.IPs[0]))
	})

	localtime := clock.Local().
		Output(time.Second, func(now time.Time) bar.Output {
			return outputs.Pango(
				pango.Icon("material-today").Color(colors.Scheme("dim-icon")),
				now.Format(" Mon 1/2/2006 "),
				pango.Icon("material-access-time").Color(colors.Scheme("dim-icon")),
				now.Format(" 3:04pm"),
			).OnClick(click.RunLeft("gsimplecal"))
		})

	wifiName, wifiDetails := split.New(wlan.Any().Output(func(i wlan.Info) bar.Output {
		if !i.Connecting() && !i.Connected() {
			mainModalController.SetOutput("network", makeIconOutput("mdi-ethernet"))
			return nil
		}
		mainModalController.SetOutput("network", makeIconOutput("mdi-wifi"))
		if i.Connecting() {
			return outputs.Pango(pango.Icon("mdi-wifi").Alpha(0.6), "...").
				Color(colors.Scheme("degraded"))
		}
		out := outputs.Group()
		// First segment shown in summary mode only.
		out.Append(outputs.Pango(
			pango.Icon("mdi-wifi").Alpha(0.6),
			pango.Text(truncate(i.SSID, -9)),
		).OnClick(click.Left(func() {
			mainModalController.Toggle("network")
		})))
		// Full name, frequency, bssid in detail mode
		out.Append(outputs.Pango(
			pango.Icon("mdi-wifi").Alpha(0.6),
			pango.Text(i.SSID),
		))
		out.Append(outputs.Textf("%2.1fG", i.Frequency.Gigahertz()))
		out.Append(outputs.Pango(
			pango.Icon("mdi-access-point").Alpha(0.8),
			pango.Text(i.AccessPointMAC).Small(),
		))
		return out
	}), 1)

	mainModal := modal.New()
	mainModal.Mode("network").
		SetOutput(makeIconOutput("mdi-ethernet")).
		Summary(wifiName).
		Detail(wifiDetails, net, netsp)
	// mainModal.Mode("media").
	// 	SetOutput(makeIconOutput("mdi-music-box")).
	// 	Add(vol, mediaSummary).
	// 	Detail(mediaDetail)

	var mm bar.Module
	mm, mainModalController = mainModal.Build()

	if vol != nil && mediaSummary != nil && mediaDetail != nil {
		fmt.Sprint("vol")
	}
	if wifiName != nil {
		fmt.Sprint("wifiname")
	}

	if freeMem != nil {
		fmt.Sprint("freemem")
	}

	if temp != nil {
		fmt.Sprint("temp")
	}

	panic(barista.Run(
		mm,
		freeMem,
		temp,
		localtime,
	))
}

type freegeoipResponse struct {
	Lat float64 `json:"latitude"`
	Lng float64 `json:"longitude"`
}

func whereami() (lat float64, lng float64, err error) {
	resp, err := http.Get("https://freegeoip.app/json/")
	if err != nil {
		return 0, 0, err
	}
	var res freegeoipResponse
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return 0, 0, err
	}
	return res.Lat, res.Lng, nil
}

type autoWeatherProvider struct{}

func (a autoWeatherProvider) GetWeather() (weather.Weather, error) {
	lat, lng, err := whereami()
	if err != nil {
		return weather.Weather{}, err
	}

	myweather, err := openweathermap.
		New("299b0a34b880d66c739c06433c2c0c68").
		Coords(lat, lng).
		GetWeather()

	return myweather, err
}

var startTaskManager = click.RunLeft("xfce4-taskmanager")

func home(path string) string {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}
	return filepath.Join(usr.HomeDir, path)
}
