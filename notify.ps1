# One Windows notification, on behalf of notify.go.
#
# This file is a text/template: notify.go fills in the launch address, the notifier and the two
# lines of text, then hands the result to powershell.exe as -EncodedCommand. That is also why it
# takes no parameters - a parameter would have to travel on the command line, through the
# process's ANSI code page, where a Chinese title is not the same title any more. This file
# itself stays ASCII: the only thing that may not be is the text, and that arrives at run time.
#
# The payload is written out here, next to the only code that consumes it. It sits inside a
# single-quoted PowerShell string, so it must not contain an apostrophe - which is what the
# template's xml function is for: it escapes the page's own text, an apostrophe included.
#
# The notifier is this app's AppUserModelID. The fallback is PowerShell's own, always registered:
# a notification still appears, signed as Windows PowerShell, if the app's registration did not
# happen.
#
# The picture at the top left is not in here: that slot is filled by the app registration
# (notify.go). An image in this payload would become appLogoOverride, which is the thumbnail
# beside the body - a different place, and not the one this app wants it in.
#
# The newlines inside the payload are for reading. Whitespace between elements is not content,
# and Windows.Data.Xml.Dom drops it.

[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml('<toast activationType="protocol" launch="{{ .Launch }}">
  <visual>
    <binding template="ToastGeneric">
      <text>{{ xml .Title }}</text>
      <text>{{ xml .Body }}</text>
    </binding>
  </visual>
</toast>')

$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
try { 
    [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{{ .Notifier }}').Show($toast) 
} 
catch { 
    [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe').Show($toast)
}
