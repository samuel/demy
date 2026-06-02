# Weather condition icons

These are the **static monochrome** [Meteocons](https://bas.dev/work/meteocons) icons by
Bas Milius, taken from the npm package [`@meteocons/svg-static@0.1.0`](https://www.npmjs.com/package/@meteocons/svg-static)
(`monochrome/` set). Licensed MIT — see `LICENSE`.

They are embedded and rasterized to bitmaps at startup by `../weathericons.go`. All source
SVGs use `viewBox="0 0 128 128"` and `currentColor`, which the rasterizer resolves to black
on a white background.

## Filename → Meteocons source mapping

Files are renamed to match the keys returned by `conditionIconKey` (`../weather.go`):

| file (key)                 | Meteocons source       |
| -------------------------- | ---------------------- |
| `weather-sunny`            | `clear-day`            |
| `weather-clear-night`      | `clear-night`          |
| `weather-partlycloudy`     | `partly-cloudy-day` \* |
| `weather-cloudy`           | `cloudy`               |
| `weather-fog`              | `fog`                  |
| `weather-rainy`            | `rain`                 |
| `weather-snowy`            | `snow`                 |
| `weather-hail`             | `hail`                 |
| `weather-lightning`        | `thunderstorms`        |
| `weather-windy`            | `wind`                 |
| `weather-exceptional`      | `code-red`             |

## Adaptations for the oksvg rasterizer

The rasterizer (`github.com/srwiley/oksvg`) does **not** support `<mask>` elements, and its
parser does not skip the subtree of an unsupported element — so a mask's full-canvas
"Subtract" path would otherwise be drawn as a near-solid black square.

\* `weather-partlycloudy.svg` originally wrapped the sun in a `<mask>` so it wouldn't show
through the cloud. The cloud is an opaque black shape that covers the sun anyway, so the
mask block was removed; the silhouette is unchanged. (This is why `partly-cloudy-day` was
chosen over `code-red`-style alternatives — `code-red` is mask-free, hence the
`weather-exceptional` mapping.)

Clip-paths are left intact: they are defined inside `<defs>` (which oksvg collects but does
not draw) and reference a full-canvas rect, so they have no visual effect either way.

To add or update an icon, prefer a `monochrome/` source with no `<mask>` element. If a chosen
icon uses a mask, strip the `<mask>…</mask>` block as above and verify with the weather icon
test that it still rasterizes correctly.
