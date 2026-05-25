package main

var tutorialLessons = []lesson{
	{
		ID:    1,
		Title: "Hello, World",
		Desc:  "Every demy layout starts with a root node. Here a VStack holds a single Text node. VStack stacks children top-to-bottom. Padding adds space inside the container. Try changing the text or font size.",
		GoEquiv: `demy.VStack(
    demy.Padding(16),
    demy.Text("Hello, World!", demy.Style{
        Font: "sans",
        Size: 24,
    }),
)`,
		SceneXML: `<vstack padding="16">
	<text font="sans" size="24">Hello, World!</text>
</vstack>`,
	},
	{
		ID:    2,
		Title: "Text Styles",
		Desc:  "The Style struct controls font size, weight, alignment, and color. Weights range from \"thin\" through \"black\". Colors are hex strings like \"#555555\". Try changing size, weight, and align on different text nodes.",
		GoEquiv: `demy.VStack(
    demy.Padding(24), demy.Gap(12),
    demy.Text("Regular", demy.Style{Font: "sans", Size: 16}),
    demy.Text("Bold", demy.Style{Font: "sans", Size: 16, Weight: demy.FontBold}),
    demy.Text("Large", demy.Style{Font: "sans", Size: 32, Weight: demy.FontBold}),
    demy.Text("Light", demy.Style{Font: "sans", Size: 14, Weight: demy.FontLight}),
    demy.Text("Centered", demy.Style{Font: "sans", Size: 16, Align: demy.TextAlignCenter}),
    demy.Text("Right", demy.Style{Font: "sans", Size: 16, Align: demy.TextAlignRight}),
    demy.Text("Gray", demy.Style{Font: "sans", Size: 16, Color: color.RGBA{R:85,G:85,B:85,A:255}}),
)`,
		SceneXML: `<vstack padding="24" gap="12">
	<text font="sans" size="16">Regular — default weight, 16pt</text>
	<text font="sans" size="16" weight="bold">Bold — weight: "bold", 16pt</text>
	<text font="sans" size="32" weight="bold">Large heading — 32pt bold</text>
	<text font="sans" size="14" weight="light">Light — weight: "light", 14pt</text>
	<text font="sans" size="16" align="center">Centered — align: "center"</text>
	<text font="sans" size="16" align="right">Right-aligned — align: "right"</text>
	<text font="sans" size="16" color="#555555">Custom color — #555555</text>
</vstack>`,
	},
	{
		ID:    3,
		Title: "VStack",
		Desc:  "VStack arranges children vertically. gap controls space between children. padding adds inset inside the stack. Try adjusting gap from 4 to 32, or adding more children.",
		GoEquiv: `demy.VStack(
    demy.Padding(24),
    demy.Gap(16),
    demy.Text("Item One",   style),
    demy.Text("Item Two",   style),
    demy.Text("Item Three", style),
    demy.Text("Item Four",  style),
)`,
		SceneXML: `<vstack padding="24" gap="16">
	<text font="sans" size="18" weight="bold">Item One</text>
	<text font="sans" size="18" weight="bold">Item Two</text>
	<text font="sans" size="18" weight="bold">Item Three</text>
	<text font="sans" size="18" weight="bold">Item Four</text>
	<text font="sans" size="12" color="#999999">Try changing gap and padding — or adding more items.</text>
</vstack>`,
	},
	{
		ID:    4,
		Title: "HStack",
		Desc:  "HStack arranges children horizontally. The align field controls vertical (cross-axis) alignment: \"start\" (top), \"center\", or \"end\" (bottom). Nest HStacks and VStacks to build grid-like layouts.",
		GoEquiv: `demy.HStack(
    demy.Gap(24),
    demy.VStack(demy.Gap(4),
        demy.Text("Column A", boldStyle),
        demy.Text("Row 1", bodyStyle),
        demy.Text("Row 2", bodyStyle),
    ),
    demy.VStack(demy.Gap(4),
        demy.Text("Column B", boldStyle),
        demy.Text("Row 1", bodyStyle),
        demy.Text("Row 2", bodyStyle),
    ),
)`,
		SceneXML: `<vstack padding="24" gap="24">
	<text font="sans" size="13" color="#555555">HStack: three items side-by-side with gap: 20</text>
	<hstack gap="20">
		<text font="sans" size="22" weight="bold">Alpha</text>
		<text font="sans" size="22" weight="bold">Beta</text>
		<text font="sans" size="22" weight="bold">Gamma</text>
	</hstack>
	<text font="sans" size="13" color="#555555">Nested: VStacks inside an HStack form columns</text>
	<hstack gap="32">
		<vstack gap="6">
			<text font="sans" size="13" weight="bold">Column A</text>
			<text font="sans" size="14">Row 1</text>
			<text font="sans" size="14">Row 2</text>
			<text font="sans" size="14">Row 3</text>
		</vstack>
		<vstack gap="6">
			<text font="sans" size="13" weight="bold">Column B</text>
			<text font="sans" size="14">Row 1</text>
			<text font="sans" size="14">Row 2</text>
			<text font="sans" size="14">Row 3</text>
		</vstack>
		<vstack gap="6">
			<text font="sans" size="13" weight="bold">Column C</text>
			<text font="sans" size="14">Row 1</text>
			<text font="sans" size="14">Row 2</text>
			<text font="sans" size="14">Row 3</text>
		</vstack>
	</hstack>
</vstack>`,
	},
	{
		ID:    5,
		Title: "Spacer",
		Desc:  "Spacer() fills the remaining space in a stack, pushing adjacent siblings apart. In the header row below, it pushes the title left and date right. The vertical Spacer pushes the footer to the bottom of the screen.",
		GoEquiv: `demy.VStack(
    demy.Padding(24),
    demy.HStack(
        demy.Text("Title", boldStyle),
        demy.Spacer(),           // pushes "Date" to the right
        demy.Text("Date", small),
    ),
    demy.Spacer(),               // pushes footer to bottom
    demy.Text("Footer", small),
)`,
		SceneXML: `<vstack padding="24">
	<hstack>
		<text font="sans" size="26" weight="bold">Daily Brief</text>
		<spacer/>
		<text font="sans" size="16">Friday</text>
	</hstack>
	<text font="sans" size="13" color="#666666">Spacer() in the HStack above pushes the title and date apart.</text>
	<spacer/>
	<text font="sans" size="14" color="#444444">The vertical Spacer above pushed this text to the bottom.</text>
</vstack>`,
	},
	{
		ID:    6,
		Title: "Box",
		Desc:  "Box is a styled container with background color, border, corner radius, and padding. It can hold any child node. Try combining these properties — for example, a rounded box with a border and colored background.",
		GoEquiv: `demy.Box(
    demy.Background(demy.LightGray),
    demy.Padding(16),
    demy.Text("Gray background", style),
)

demy.Box(
    demy.Border(2, demy.Black),
    demy.CornerRadius(8),
    demy.Padding(16),
    demy.Text("Rounded border", style),
)

demy.Box(
    demy.Background(demy.Black),
    demy.Padding(16),
    demy.Text("Inverted", whiteStyle),
)`,
		SceneXML: `<vstack padding="24" gap="20">
	<box padding="16" background="#dddddd">
		<text font="sans" size="15">background: "#dddddd", padding: 16</text>
	</box>
	<box padding="16" border-width="2" border-color="#000000" corner-radius="10">
		<text font="sans" size="15">border-width: 2, corner-radius: 10</text>
	</box>
	<box padding="16" background="#000000" corner-radius="6">
		<text font="sans" size="15" color="#ffffff">Inverted: white text on black background</text>
	</box>
	<box padding="12" background="#eeeeee" border-width="1" border-color="#aaaaaa">
		<vstack gap="6">
			<text font="sans" size="14" weight="semi-bold">Box with nested VStack</text>
			<text font="sans" size="13">Children can be any Node — stacks, other boxes, text.</text>
		</vstack>
	</box>
</vstack>`,
	},
	{
		ID:    7,
		Title: "Frame",
		Desc:  "Frame(width, height) constrains its children to exact pixel dimensions. Children that are smaller are aligned within the frame; children that are larger are clipped. Try changing the frame dimensions.",
		GoEquiv: `demy.Frame(160, 120,
    demy.Box(
        demy.Background(demy.LightGray),
        demy.Padding(8),
        demy.Text("160 x 120", style),
    ),
)

demy.Frame(80, 160,
    demy.Box(
        demy.Background(demy.Black),
        demy.Padding(8),
        demy.Text("80 x 160", whiteStyle),
    ),
)`,
		SceneXML: `<vstack padding="24" gap="24">
	<text font="sans" size="14">Frame constrains children to exact pixel dimensions.</text>
	<hstack gap="16" align="center">
		<frame width="180" height="120">
			<box padding="10" background="#cccccc">
				<text font="sans" size="14">180 x 120</text>
			</box>
		</frame>
		<frame width="120" height="80">
			<box padding="10" background="#888888">
				<text font="sans" size="13" color="#ffffff">120 x 80</text>
			</box>
		</frame>
		<frame width="80" height="180">
			<box padding="10" background="#333333">
				<text font="sans" size="12" color="#ffffff">80 x 180</text>
			</box>
		</frame>
	</hstack>
</vstack>`,
	},
	{
		ID:    8,
		Title: "ZStack",
		Desc:  "ZStack layers children on top of each other. All children share the same bounds. The align field sets where children anchor within the ZStack: \"top-leading\" (default), \"center\", \"bottom\", \"bottom-trailing\", etc.",
		GoEquiv: `demy.ZStack(
    // Background layer fills bounds
    demy.Frame(600, 200,
        demy.Box(demy.Background(demy.LightGray)),
    ),
    // Overlay layer sits on top
    demy.VStack(
        demy.Padding(20), demy.Gap(8),
        demy.Text("Overlay heading", headingStyle),
        demy.Text("Layered content", bodyStyle),
    ),
)

// Center-aligned variant
demy.ZStack(demy.Center,
    demy.Frame(600, 100,
        demy.Box(demy.Background(demy.Black)),
    ),
    demy.Text("Centered on dark", whiteStyle),
)`,
		SceneXML: `<vstack padding="24" gap="24">
	<text font="sans" size="13" color="#555555">ZStack layers children. Earlier children appear behind later ones.</text>
	<zstack>
		<frame width="620" height="160">
			<box background="#cccccc"/>
		</frame>
		<vstack padding="20" gap="8">
			<text font="sans" size="20" weight="bold">Overlay: top-leading (default)</text>
			<text font="sans" size="14">This VStack sits on top of the gray Frame.</text>
		</vstack>
	</zstack>
	<zstack align="center">
		<frame width="620" height="80">
			<box background="#222222"/>
		</frame>
		<text font="sans" size="16" color="#ffffff">align: "center" — text anchored to center of the dark frame</text>
	</zstack>
</vstack>`,
	},
	{
		ID:    9,
		Title: "Divider",
		Desc:  "Divider() draws a 1px rule. In a VStack it renders horizontally; in an HStack it renders vertically. Dividers are useful for separating sections and creating column layouts.",
		GoEquiv: `// Horizontal dividers in VStack
demy.VStack(demy.Padding(24), demy.Gap(16),
    demy.Text("Section A", boldStyle),
    demy.Divider(),
    demy.Text("Section B", boldStyle),
)

// Vertical dividers in HStack
demy.HStack(demy.Padding(24), demy.Gap(16),
    demy.Text("Left",  bodyStyle),
    demy.Divider(),
    demy.Text("Right", bodyStyle),
)`,
		SceneXML: `<vstack padding="24" gap="14">
	<text font="sans" size="20" weight="bold">Section One</text>
	<text font="sans" size="13">Divider() draws a horizontal rule in a VStack.</text>
	<divider/>
	<text font="sans" size="20" weight="bold">Section Two</text>
	<text font="sans" size="13">Use it to visually separate groups of content.</text>
	<divider/>
	<text font="sans" size="20" weight="bold">Vertical Dividers in HStack</text>
	<text font="sans" size="13">In an HStack, Divider() renders as a vertical rule:</text>
	<hstack gap="16">
		<text font="sans" size="15">Left column</text>
		<divider/>
		<text font="sans" size="15">Middle column</text>
		<divider/>
		<text font="sans" size="15">Right column</text>
	</hstack>
</vstack>`,
	},
	{
		ID:    10,
		Title: "Full Layout",
		Desc:  "A realistic e-ink dashboard using style classes — defined right in the XML inside a <styles> block and referenced with class=. The <scene> wrapper lets you declare named styles once and reuse them across the layout. Try adding class=\"body\" to any text node, or override a class field inline: <text class=\"label\" size=\"16\">.",
		GoEquiv: `// Style classes in Go use Config.Styles; in XML they live in <styles>.
// Both resolve the same way: class base is merged, inline attrs override it.

demy.VStack(
    demy.Padding(20), demy.Gap(8),
    demy.HStack(
        demy.Text("Daily Brief", demy.Style{Font:"sans", Size:28, Weight:demy.FontBold}),
        demy.Spacer(),
        demy.Text("Friday, May 22", demy.Style{Font:"sans", Size:14}),
    ),
    demy.Divider(),
    demy.HStack(demy.Gap(20),
        demy.VStack(demy.Gap(6),
            demy.Text("WEATHER",          demy.Class("section-label")),
            demy.Text("72°F",             demy.Style{Font:"sans", Size:40, Weight:demy.FontBold}),
            demy.Text("Partly Cloudy",    demy.Style{Font:"sans", Size:14}),
            demy.Text("H: 78°  L: 65°",  demy.Class("label")),
        ),
        demy.Divider(),
        demy.VStack(demy.Gap(8),
            demy.Text("TODAY'S TASKS",           demy.Class("section-label")),
            demy.Text("Design review at 10am",   demy.Class("body")),
            demy.Text("Lunch with team at noon", demy.Class("body")),
            demy.Text("Deploy v2.3 release",     demy.Class("body")),
            demy.Text("Write weekly report",     demy.Class("body")),
        ),
        demy.Divider(),
        demy.Weighted(1, demy.VStack(demy.Gap(8),
            demy.Text("METRICS", demy.Class("section-label")),
            metricRow("CPU",      "42%"),
            metricRow("Memory",   "3.1 GB"),
            metricRow("Requests", "1,247/s"),
            metricRow("Uptime",   "14d 3h"),
        )),
    ),
    demy.Spacer(),
    demy.Box(demy.Background(demy.Black), demy.Padding(8), demy.CornerRadius(4),
        demy.Text("Last updated 9:00 AM",
            demy.Style{Font:"sans", Size:11, Color:demy.White, Align:demy.TextAlignCenter}),
    ),
)

// Class("label") and Class("value") — style defined once, used everywhere.
func metricRow(label, value string) demy.Node {
    return demy.HStack(
        demy.Text(label, demy.Class("label")),
        demy.Spacer(),
        demy.Text(value, demy.Class("value")),
    )
}`,
		SceneXML: `<scene>
<styles>
	<style name="section-label" font="sans" size="11" weight="semi-bold" color="#777777"/>
	<style name="body" font="sans" size="13"/>
	<style name="label" font="sans" size="13" color="#555555"/>
	<style name="value" font="sans" size="13" weight="bold" align="right"/>
</styles>
<vstack padding="20" gap="8">
	<hstack>
		<text font="sans" size="28" weight="bold">Daily Brief</text>
		<spacer/>
		<text font="sans" size="14">Friday, May 22</text>
	</hstack>
	<divider/>
	<hstack gap="20">
		<vstack gap="6">
			<text class="section-label">WEATHER</text>
			<text font="sans" size="40" weight="bold">72°F</text>
			<text font="sans" size="14">Partly Cloudy</text>
			<text class="label">H: 78°  L: 65°</text>
		</vstack>
		<divider/>
		<vstack gap="8">
			<text class="section-label">TODAY'S TASKS</text>
			<text class="body">Design review at 10am</text>
			<text class="body">Lunch with team at noon</text>
			<text class="body">Deploy v2.3 release</text>
			<text class="body">Write weekly report</text>
		</vstack>
		<divider/>
		<weighted weight="1">
			<vstack gap="8">
				<text class="section-label">METRICS</text>
				<hstack>
					<text class="label">CPU</text>
					<spacer/>
					<text class="value">42%</text>
				</hstack>
				<hstack>
					<text class="label">Memory</text>
					<spacer/>
					<text class="value">3.1 GB</text>
				</hstack>
				<hstack>
					<text class="label">Requests</text>
					<spacer/>
					<text class="value">1,247/s</text>
				</hstack>
				<hstack>
					<text class="label">Uptime</text>
					<spacer/>
					<text class="value">14d 3h</text>
				</hstack>
			</vstack>
		</weighted>
	</hstack>
	<spacer/>
	<box background="#000000" padding="8" corner-radius="4">
		<text font="sans" size="11" color="#ffffff" align="center">Last updated 9:00 AM — refreshes every 5 minutes</text>
	</box>
</vstack>
</scene>`,
	},
}
