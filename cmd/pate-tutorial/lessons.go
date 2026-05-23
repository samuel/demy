package main

var tutorialLessons = []lesson{
	{
		ID:    1,
		Title: "Hello, World",
		Desc:  "Every pate layout starts with a root node. Here a VStack holds a single Text node. VStack stacks children top-to-bottom. Padding adds space inside the container. Try changing the text or font size.",
		GoEquiv: `pate.VStack(
    pate.Padding(16),
    pate.Text("Hello, World!", pate.Style{
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
		GoEquiv: `pate.VStack(
    pate.Padding(24), pate.Gap(12),
    pate.Text("Regular", pate.Style{Font: "sans", Size: 16}),
    pate.Text("Bold", pate.Style{Font: "sans", Size: 16, Weight: pate.FontBold}),
    pate.Text("Large", pate.Style{Font: "sans", Size: 32, Weight: pate.FontBold}),
    pate.Text("Light", pate.Style{Font: "sans", Size: 14, Weight: pate.FontLight}),
    pate.Text("Centered", pate.Style{Font: "sans", Size: 16, Align: pate.TextAlignCenter}),
    pate.Text("Right", pate.Style{Font: "sans", Size: 16, Align: pate.TextAlignRight}),
    pate.Text("Gray", pate.Style{Font: "sans", Size: 16, Color: color.RGBA{R:85,G:85,B:85,A:255}}),
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
		GoEquiv: `pate.VStack(
    pate.Padding(24),
    pate.Gap(16),
    pate.Text("Item One",   style),
    pate.Text("Item Two",   style),
    pate.Text("Item Three", style),
    pate.Text("Item Four",  style),
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
		GoEquiv: `pate.HStack(
    pate.Gap(24),
    pate.VStack(pate.Gap(4),
        pate.Text("Column A", boldStyle),
        pate.Text("Row 1", bodyStyle),
        pate.Text("Row 2", bodyStyle),
    ),
    pate.VStack(pate.Gap(4),
        pate.Text("Column B", boldStyle),
        pate.Text("Row 1", bodyStyle),
        pate.Text("Row 2", bodyStyle),
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
		GoEquiv: `pate.VStack(
    pate.Padding(24),
    pate.HStack(
        pate.Text("Title", boldStyle),
        pate.Spacer(),           // pushes "Date" to the right
        pate.Text("Date", small),
    ),
    pate.Spacer(),               // pushes footer to bottom
    pate.Text("Footer", small),
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
		GoEquiv: `pate.Box(
    pate.Background(pate.LightGray),
    pate.Padding(16),
    pate.Text("Gray background", style),
)

pate.Box(
    pate.Border(2, pate.Black),
    pate.CornerRadius(8),
    pate.Padding(16),
    pate.Text("Rounded border", style),
)

pate.Box(
    pate.Background(pate.Black),
    pate.Padding(16),
    pate.Text("Inverted", whiteStyle),
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
		GoEquiv: `pate.Frame(160, 120,
    pate.Box(
        pate.Background(pate.LightGray),
        pate.Padding(8),
        pate.Text("160 x 120", style),
    ),
)

pate.Frame(80, 160,
    pate.Box(
        pate.Background(pate.Black),
        pate.Padding(8),
        pate.Text("80 x 160", whiteStyle),
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
		GoEquiv: `pate.ZStack(
    // Background layer fills bounds
    pate.Frame(600, 200,
        pate.Box(pate.Background(pate.LightGray)),
    ),
    // Overlay layer sits on top
    pate.VStack(
        pate.Padding(20), pate.Gap(8),
        pate.Text("Overlay heading", headingStyle),
        pate.Text("Layered content", bodyStyle),
    ),
)

// Center-aligned variant
pate.ZStack(pate.Center,
    pate.Frame(600, 100,
        pate.Box(pate.Background(pate.Black)),
    ),
    pate.Text("Centered on dark", whiteStyle),
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
pate.VStack(pate.Padding(24), pate.Gap(16),
    pate.Text("Section A", boldStyle),
    pate.Divider(),
    pate.Text("Section B", boldStyle),
)

// Vertical dividers in HStack
pate.HStack(pate.Padding(24), pate.Gap(16),
    pate.Text("Left",  bodyStyle),
    pate.Divider(),
    pate.Text("Right", bodyStyle),
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
		Desc:  "A realistic e-ink dashboard combining VStack, HStack, ZStack, Box, Spacer, Weighted, Divider, and Text. The Metrics column uses Weighted(1, …) to claim all remaining horizontal space after the fixed-width Weather and Tasks columns. Inside each metric row, the value text is itself Weighted so it fills the remaining row width and right-aligns flush to the column edge.",
		GoEquiv: `pate.VStack(
    pate.Padding(20), pate.Gap(8),
    // Header row
    pate.HStack(
        pate.Text("Daily Brief", headingStyle),
        pate.Spacer(),
        pate.Text("Friday, May 22", smallStyle),
    ),
    pate.Divider(),
    // Three-column content — Metrics column claims leftover space
    pate.HStack(pate.Gap(20),
        weatherColumn,
        pate.Divider(),
        tasksColumn,
        pate.Divider(),
        pate.Weighted(1, metricsColumn),
    ),
    pate.Spacer(),
    // Footer badge
    pate.Box(pate.Background(pate.Black), pate.Padding(8), pate.CornerRadius(4),
        pate.Text("Last updated 9:00 AM", whiteCenter),
    ),
)

// metric row helper — label left, value right-aligned
func metricRow(label, value string) pate.Node {
    return pate.HStack(
        pate.Text(label, labelStyle),
        pate.Spacer(),
        pate.Text(value, pate.Style{
            Font: "sans", Size: 13, Weight: pate.FontBold,
            Align: pate.TextAlignRight,
        }),
    )
}`,
		SceneXML: `<vstack padding="20" gap="8">
	<hstack>
		<text font="sans" size="28" weight="bold">Daily Brief</text>
		<spacer/>
		<text font="sans" size="14">Friday, May 22</text>
	</hstack>
	<divider/>
	<hstack gap="20">
		<vstack gap="6">
			<text font="sans" size="11" weight="semi-bold" color="#777777">WEATHER</text>
			<text font="sans" size="40" weight="bold">72°F</text>
			<text font="sans" size="14">Partly Cloudy</text>
			<text font="sans" size="13" color="#555555">H: 78°  L: 65°</text>
		</vstack>
		<divider/>
		<vstack gap="8">
			<text font="sans" size="11" weight="semi-bold" color="#777777">TODAY'S TASKS</text>
			<text font="sans" size="13">Design review at 10am</text>
			<text font="sans" size="13">Lunch with team at noon</text>
			<text font="sans" size="13">Deploy v2.3 release</text>
			<text font="sans" size="13">Write weekly report</text>
		</vstack>
		<divider/>
		<weighted weight="1">
			<vstack gap="8">
				<text font="sans" size="11" weight="semi-bold" color="#777777">METRICS</text>
				<hstack>
					<text font="sans" size="13" color="#555555">CPU</text>
					<spacer/>
					<text font="sans" size="13" weight="bold" align="right">42%</text>
				</hstack>
				<hstack>
					<text font="sans" size="13" color="#555555">Memory</text>
					<spacer/>
					<text font="sans" size="13" weight="bold" align="right">3.1 GB</text>
				</hstack>
				<hstack>
					<text font="sans" size="13" color="#555555">Requests</text>
					<spacer/>
					<text font="sans" size="13" weight="bold" align="right">1,247/s</text>
				</hstack>
				<hstack>
					<text font="sans" size="13" color="#555555">Uptime</text>
					<spacer/>
					<text font="sans" size="13" weight="bold" align="right">14d 3h</text>
				</hstack>
			</vstack>
		</weighted>
	</hstack>
	<spacer/>
	<box background="#000000" padding="8" corner-radius="4">
		<text font="sans" size="11" color="#ffffff" align="center">Last updated 9:00 AM — refreshes every 5 minutes</text>
	</box>
</vstack>`,
	},
}
