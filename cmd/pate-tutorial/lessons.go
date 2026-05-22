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
		SceneJSON: `{
  "type": "vstack",
  "padding": 16,
  "children": [
    {
      "type": "text",
      "text": "Hello, World!",
      "style": {
        "font": "sans",
        "size": 24
      }
    }
  ]
}`,
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
		SceneJSON: `{
  "type": "vstack",
  "padding": 24,
  "gap": 12,
  "children": [
    {
      "type": "text",
      "text": "Regular — default weight, 16pt",
      "style": { "font": "sans", "size": 16 }
    },
    {
      "type": "text",
      "text": "Bold — weight: \"bold\", 16pt",
      "style": { "font": "sans", "size": 16, "weight": "bold" }
    },
    {
      "type": "text",
      "text": "Large heading — 32pt bold",
      "style": { "font": "sans", "size": 32, "weight": "bold" }
    },
    {
      "type": "text",
      "text": "Light — weight: \"light\", 14pt",
      "style": { "font": "sans", "size": 14, "weight": "light" }
    },
    {
      "type": "text",
      "text": "Centered — align: \"center\"",
      "style": { "font": "sans", "size": 16, "align": "center" }
    },
    {
      "type": "text",
      "text": "Right-aligned — align: \"right\"",
      "style": { "font": "sans", "size": 16, "align": "right" }
    },
    {
      "type": "text",
      "text": "Custom color — #555555",
      "style": { "font": "sans", "size": 16, "color": "#555555" }
    }
  ]
}`,
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
		SceneJSON: `{
  "type": "vstack",
  "padding": 24,
  "gap": 16,
  "children": [
    {
      "type": "text",
      "text": "Item One",
      "style": { "font": "sans", "size": 18, "weight": "bold" }
    },
    {
      "type": "text",
      "text": "Item Two",
      "style": { "font": "sans", "size": 18, "weight": "bold" }
    },
    {
      "type": "text",
      "text": "Item Three",
      "style": { "font": "sans", "size": 18, "weight": "bold" }
    },
    {
      "type": "text",
      "text": "Item Four",
      "style": { "font": "sans", "size": 18, "weight": "bold" }
    },
    {
      "type": "text",
      "text": "Try changing gap and padding — or adding more items.",
      "style": { "font": "sans", "size": 12, "color": "#999999" }
    }
  ]
}`,
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
		SceneJSON: `{
  "type": "vstack",
  "padding": 24,
  "gap": 24,
  "children": [
    {
      "type": "text",
      "text": "HStack: three items side-by-side with gap: 20",
      "style": { "font": "sans", "size": 13, "color": "#555555" }
    },
    {
      "type": "hstack",
      "gap": 20,
      "children": [
        { "type": "text", "text": "Alpha", "style": { "font": "sans", "size": 22, "weight": "bold" } },
        { "type": "text", "text": "Beta",  "style": { "font": "sans", "size": 22, "weight": "bold" } },
        { "type": "text", "text": "Gamma", "style": { "font": "sans", "size": 22, "weight": "bold" } }
      ]
    },
    {
      "type": "text",
      "text": "Nested: VStacks inside an HStack form columns",
      "style": { "font": "sans", "size": 13, "color": "#555555" }
    },
    {
      "type": "hstack",
      "gap": 32,
      "children": [
        {
          "type": "vstack",
          "gap": 6,
          "children": [
            { "type": "text", "text": "Column A", "style": { "font": "sans", "size": 13, "weight": "bold" } },
            { "type": "text", "text": "Row 1", "style": { "font": "sans", "size": 14 } },
            { "type": "text", "text": "Row 2", "style": { "font": "sans", "size": 14 } },
            { "type": "text", "text": "Row 3", "style": { "font": "sans", "size": 14 } }
          ]
        },
        {
          "type": "vstack",
          "gap": 6,
          "children": [
            { "type": "text", "text": "Column B", "style": { "font": "sans", "size": 13, "weight": "bold" } },
            { "type": "text", "text": "Row 1", "style": { "font": "sans", "size": 14 } },
            { "type": "text", "text": "Row 2", "style": { "font": "sans", "size": 14 } },
            { "type": "text", "text": "Row 3", "style": { "font": "sans", "size": 14 } }
          ]
        },
        {
          "type": "vstack",
          "gap": 6,
          "children": [
            { "type": "text", "text": "Column C", "style": { "font": "sans", "size": 13, "weight": "bold" } },
            { "type": "text", "text": "Row 1", "style": { "font": "sans", "size": 14 } },
            { "type": "text", "text": "Row 2", "style": { "font": "sans", "size": 14 } },
            { "type": "text", "text": "Row 3", "style": { "font": "sans", "size": 14 } }
          ]
        }
      ]
    }
  ]
}`,
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
		SceneJSON: `{
  "type": "vstack",
  "padding": 24,
  "children": [
    {
      "type": "hstack",
      "children": [
        {
          "type": "text",
          "text": "Daily Brief",
          "style": { "font": "sans", "size": 26, "weight": "bold" }
        },
        { "type": "spacer" },
        {
          "type": "text",
          "text": "Friday",
          "style": { "font": "sans", "size": 16 }
        }
      ]
    },
    {
      "type": "text",
      "text": "Spacer() in the HStack above pushes the title and date apart.",
      "style": { "font": "sans", "size": 13, "color": "#666666" }
    },
    { "type": "spacer" },
    {
      "type": "text",
      "text": "The vertical Spacer above pushed this text to the bottom.",
      "style": { "font": "sans", "size": 14, "color": "#444444" }
    }
  ]
}`,
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
		SceneJSON: `{
  "type": "vstack",
  "padding": 24,
  "gap": 20,
  "children": [
    {
      "type": "box",
      "padding": 16,
      "background": "#dddddd",
      "children": [
        { "type": "text", "text": "background: \"#dddddd\", padding: 16", "style": { "font": "sans", "size": 15 } }
      ]
    },
    {
      "type": "box",
      "padding": 16,
      "border_width": 2,
      "border_color": "#000000",
      "corner_radius": 10,
      "children": [
        { "type": "text", "text": "border_width: 2, corner_radius: 10", "style": { "font": "sans", "size": 15 } }
      ]
    },
    {
      "type": "box",
      "padding": 16,
      "background": "#000000",
      "corner_radius": 6,
      "children": [
        { "type": "text", "text": "Inverted: white text on black background", "style": { "font": "sans", "size": 15, "color": "#ffffff" } }
      ]
    },
    {
      "type": "box",
      "padding": 12,
      "background": "#eeeeee",
      "border_width": 1,
      "border_color": "#aaaaaa",
      "children": [
        {
          "type": "vstack",
          "gap": 6,
          "children": [
            { "type": "text", "text": "Box with nested VStack", "style": { "font": "sans", "size": 14, "weight": "semi-bold" } },
            { "type": "text", "text": "Children can be any Node — stacks, other boxes, text.", "style": { "font": "sans", "size": 13 } }
          ]
        }
      ]
    }
  ]
}`,
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
		SceneJSON: `{
  "type": "vstack",
  "padding": 24,
  "gap": 24,
  "children": [
    {
      "type": "text",
      "text": "Frame constrains children to exact pixel dimensions.",
      "style": { "font": "sans", "size": 14 }
    },
    {
      "type": "hstack",
      "gap": 16,
      "align": "center",
      "children": [
        {
          "type": "frame",
          "width": 180,
          "height": 120,
          "children": [
            {
              "type": "box",
              "padding": 10,
              "background": "#cccccc",
              "children": [
                { "type": "text", "text": "180 x 120", "style": { "font": "sans", "size": 14 } }
              ]
            }
          ]
        },
        {
          "type": "frame",
          "width": 120,
          "height": 80,
          "children": [
            {
              "type": "box",
              "padding": 10,
              "background": "#888888",
              "children": [
                { "type": "text", "text": "120 x 80", "style": { "font": "sans", "size": 13, "color": "#ffffff" } }
              ]
            }
          ]
        },
        {
          "type": "frame",
          "width": 80,
          "height": 180,
          "children": [
            {
              "type": "box",
              "padding": 10,
              "background": "#333333",
              "children": [
                { "type": "text", "text": "80 x 180", "style": { "font": "sans", "size": 12, "color": "#ffffff" } }
              ]
            }
          ]
        }
      ]
    }
  ]
}`,
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
		SceneJSON: `{
  "type": "vstack",
  "padding": 24,
  "gap": 24,
  "children": [
    {
      "type": "text",
      "text": "ZStack layers children. Earlier children appear behind later ones.",
      "style": { "font": "sans", "size": 13, "color": "#555555" }
    },
    {
      "type": "zstack",
      "children": [
        {
          "type": "frame",
          "width": 620,
          "height": 160,
          "children": [
            { "type": "box", "background": "#cccccc" }
          ]
        },
        {
          "type": "vstack",
          "padding": 20,
          "gap": 8,
          "children": [
            { "type": "text", "text": "Overlay: top-leading (default)", "style": { "font": "sans", "size": 20, "weight": "bold" } },
            { "type": "text", "text": "This VStack sits on top of the gray Frame.", "style": { "font": "sans", "size": 14 } }
          ]
        }
      ]
    },
    {
      "type": "zstack",
      "align": "center",
      "children": [
        {
          "type": "frame",
          "width": 620,
          "height": 80,
          "children": [
            { "type": "box", "background": "#222222" }
          ]
        },
        {
          "type": "text",
          "text": "align: \"center\" — text anchored to center of the dark frame",
          "style": { "font": "sans", "size": 16, "color": "#ffffff" }
        }
      ]
    }
  ]
}`,
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
		SceneJSON: `{
  "type": "vstack",
  "padding": 24,
  "gap": 14,
  "children": [
    { "type": "text", "text": "Section One", "style": { "font": "sans", "size": 20, "weight": "bold" } },
    { "type": "text", "text": "Divider() draws a horizontal rule in a VStack.", "style": { "font": "sans", "size": 13 } },
    { "type": "divider" },
    { "type": "text", "text": "Section Two", "style": { "font": "sans", "size": 20, "weight": "bold" } },
    { "type": "text", "text": "Use it to visually separate groups of content.", "style": { "font": "sans", "size": 13 } },
    { "type": "divider" },
    { "type": "text", "text": "Vertical Dividers in HStack", "style": { "font": "sans", "size": 20, "weight": "bold" } },
    { "type": "text", "text": "In an HStack, Divider() renders as a vertical rule:", "style": { "font": "sans", "size": 13 } },
    {
      "type": "hstack",
      "gap": 16,
      "children": [
        { "type": "text", "text": "Left column", "style": { "font": "sans", "size": 15 } },
        { "type": "divider" },
        { "type": "text", "text": "Middle column", "style": { "font": "sans", "size": 15 } },
        { "type": "divider" },
        { "type": "text", "text": "Right column", "style": { "font": "sans", "size": 15 } }
      ]
    }
  ]
}`,
	},
	{
		ID:    10,
		Title: "Full Layout",
		Desc:  "A realistic e-ink dashboard combining VStack, HStack, ZStack, Box, Spacer, Divider, and Text. This is the kind of layout you'd deploy to a TRMNL device. Try customizing the text, adding metrics, or adjusting the visual hierarchy.",
		GoEquiv: `pate.VStack(
    pate.Padding(20), pate.Gap(8),
    // Header row
    pate.HStack(
        pate.Text("Daily Brief", headingStyle),
        pate.Spacer(),
        pate.Text("Friday, May 22", smallStyle),
    ),
    pate.Divider(),
    // Three-column content
    pate.HStack(pate.Gap(20),
        weatherColumn,
        pate.Divider(),
        tasksColumn,
        pate.Divider(),
        metricsColumn,
    ),
    pate.Spacer(),
    // Footer badge
    pate.Box(pate.Background(pate.Black), pate.Padding(8), pate.CornerRadius(4),
        pate.Text("Last updated 9:00 AM", whiteCenter),
    ),
)`,
		SceneJSON: `{
  "type": "vstack",
  "padding": 20,
  "gap": 8,
  "children": [
    {
      "type": "hstack",
      "children": [
        { "type": "text", "text": "Daily Brief", "style": { "font": "sans", "size": 28, "weight": "bold" } },
        { "type": "spacer" },
        { "type": "text", "text": "Friday, May 22", "style": { "font": "sans", "size": 14 } }
      ]
    },
    { "type": "divider" },
    {
      "type": "hstack",
      "gap": 20,
      "children": [
        {
          "type": "vstack",
          "gap": 6,
          "children": [
            { "type": "text", "text": "WEATHER", "style": { "font": "sans", "size": 11, "weight": "semi-bold", "color": "#777777" } },
            { "type": "text", "text": "72°F", "style": { "font": "sans", "size": 40, "weight": "bold" } },
            { "type": "text", "text": "Partly Cloudy", "style": { "font": "sans", "size": 14 } },
            { "type": "text", "text": "H: 78°  L: 65°", "style": { "font": "sans", "size": 13, "color": "#555555" } }
          ]
        },
        { "type": "divider" },
        {
          "type": "vstack",
          "gap": 8,
          "children": [
            { "type": "text", "text": "TODAY'S TASKS", "style": { "font": "sans", "size": 11, "weight": "semi-bold", "color": "#777777" } },
            { "type": "text", "text": "Design review at 10am", "style": { "font": "sans", "size": 13 } },
            { "type": "text", "text": "Lunch with team at noon", "style": { "font": "sans", "size": 13 } },
            { "type": "text", "text": "Deploy v2.3 release", "style": { "font": "sans", "size": 13 } },
            { "type": "text", "text": "Write weekly report", "style": { "font": "sans", "size": 13 } }
          ]
        },
        { "type": "divider" },
        {
          "type": "vstack",
          "gap": 8,
          "children": [
            { "type": "text", "text": "METRICS", "style": { "font": "sans", "size": 11, "weight": "semi-bold", "color": "#777777" } },
            {
              "type": "hstack",
              "children": [
                { "type": "text", "text": "CPU", "style": { "font": "sans", "size": 13, "color": "#555555" } },
                { "type": "spacer" },
                { "type": "text", "text": "42%", "style": { "font": "sans", "size": 13, "weight": "bold" } }
              ]
            },
            {
              "type": "hstack",
              "children": [
                { "type": "text", "text": "Memory", "style": { "font": "sans", "size": 13, "color": "#555555" } },
                { "type": "spacer" },
                { "type": "text", "text": "3.1 GB", "style": { "font": "sans", "size": 13, "weight": "bold" } }
              ]
            },
            {
              "type": "hstack",
              "children": [
                { "type": "text", "text": "Requests", "style": { "font": "sans", "size": 13, "color": "#555555" } },
                { "type": "spacer" },
                { "type": "text", "text": "1,247/s", "style": { "font": "sans", "size": 13, "weight": "bold" } }
              ]
            },
            {
              "type": "hstack",
              "children": [
                { "type": "text", "text": "Uptime", "style": { "font": "sans", "size": 13, "color": "#555555" } },
                { "type": "spacer" },
                { "type": "text", "text": "14d 3h", "style": { "font": "sans", "size": 13, "weight": "bold" } }
              ]
            }
          ]
        }
      ]
    },
    { "type": "spacer" },
    {
      "type": "box",
      "background": "#000000",
      "padding": 8,
      "corner_radius": 4,
      "children": [
        { "type": "text", "text": "Last updated 9:00 AM — refreshes every 5 minutes", "style": { "font": "sans", "size": 11, "color": "#ffffff", "align": "center" } }
      ]
    }
  ]
}`,
	},
}
