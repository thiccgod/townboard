$img="$env:TEMP\townboard-tmp.jpg"

Add-Type -AssemblyName System.Windows.Forms
$screens=[System.Windows.Forms.Screen]::AllScreens

Add-Type -AssemblyName System.Drawing
function screenshot($s) {
    $w=$s.Bounds.Width
    $h=$s.Bounds.Height
    $x=$s.Bounds.X
    $y=$s.Bounds.Y

    $b=new-object System.Drawing.Bitmap $w, $h
    $g=[System.Drawing.Graphics]::FromImage($b)
    $g.CopyFromScreen($x, $y, 0, 0, $b.Size)

    $b.Save($img)
}

foreach ($screen in $screens)
{
    if ($screen.Primary -eq $TRUE ) {
        screenshot($screen)
    }
}
