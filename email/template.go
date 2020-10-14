package email

import (
	"html/template"
	"log"
)

var DefaultTemplate *template.Template

func init() {
	var err error
	DefaultTemplate, err = template.New("email").Parse(defaultTemplate)
	if err != nil {
		log.Fatal("failed to parse email template: " + err.Error())
	}
}

var defaultTemplate = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html data-editor-version="2" class="sg-campaigns" xmlns="http://www.w3.org/1999/xhtml">
<head>
    <meta http-equiv="Content-Type" content="text/html; charset=utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, minimum-scale=1, maximum-scale=1">
    <!--[if !mso]><!-->
    <meta http-equiv="X-UA-Compatible" content="IE=Edge">
    <!--<![endif]-->
    <!--[if (gte mso 9)|(IE)]>
    <xml>
        <o:OfficeDocumentSettings>
            <o:AllowPNG/>
            <o:PixelsPerInch>96</o:PixelsPerInch>
        </o:OfficeDocumentSettings>
    </xml>
    <![endif]-->
    <!--[if (gte mso 9)|(IE)]>
    <style type="text/css">
        body {width: 600px;margin: 0 auto;}
        table {border-collapse: collapse;}
        table, td {mso-table-lspace: 0pt;mso-table-rspace: 0pt;}
        img {-ms-interpolation-mode: bicubic;}
    </style>
    <![endif]-->
    <style type="text/css">
        body, p, div {
            font-family: verdana,geneva,sans-serif;
            font-size: 16px;
        }
        body {
            color: #516775;
        }
        body a {
            color: #993300;
            text-decoration: none;
        }
        p { margin: 0; padding: 0; }
        table.wrapper {
            width:100% !important;
            table-layout: fixed;
            -webkit-font-smoothing: antialiased;
            -webkit-text-size-adjust: 100%;
            -moz-text-size-adjust: 100%;
            -ms-text-size-adjust: 100%;
        }
        img.max-width {
            max-width: 100% !important;
        }
        .column.of-2 {
            width: 50%;
        }
        .column.of-3 {
            width: 33.333%;
        }
        .column.of-4 {
            width: 25%;
        }
        @media screen and (max-width:480px) {
            .preheader .rightColumnContent,
            .footer .rightColumnContent {
                text-align: left !important;
            }
            .preheader .rightColumnContent div,
            .preheader .rightColumnContent span,
            .footer .rightColumnContent div,
            .footer .rightColumnContent span {
                text-align: left !important;
            }
            .preheader .rightColumnContent,
            .preheader .leftColumnContent {
                font-size: 80% !important;
                padding: 5px 0;
            }
            table.wrapper-mobile {
                width: 100% !important;
                table-layout: fixed;
            }
            img.max-width {
                height: auto !important;
                max-width: 100% !important;
            }
            a.bulletproof-button {
                display: block !important;
                width: auto !important;
                font-size: 80%;
                padding-left: 0 !important;
                padding-right: 0 !important;
            }
            .columns {
                width: 100% !important;
            }
            .column {
                display: block !important;
                width: 100% !important;
                padding-left: 0 !important;
                padding-right: 0 !important;
                margin-left: 0 !important;
                margin-right: 0 !important;
            }
            .social-icon-column {
                display: inline-block !important;
            }
        }
    </style>
    <!--user entered Head Start-->

    <!--End Head user entered-->
</head>
<body>
<center class="wrapper" data-link-color="#993300" data-body-style="font-size:16px; font-family:verdana,geneva,sans-serif; color:#516775; background-color:#F9F5F2;">
    <div class="webkit">
        <table cellpadding="0" cellspacing="0" border="0" width="100%" class="wrapper" bgcolor="#F9F5F2">
            <tr>
                <td valign="top" bgcolor="#F9F5F2" width="100%">
                    <table width="100%" role="content-container" class="outer" align="center" cellpadding="0" cellspacing="0" border="0">
                        <tr>
                            <td width="100%">
                                <table width="100%" cellpadding="0" cellspacing="0" border="0">
                                    <tr>
                                        <td>
                                            <!--[if mso]>
                                            <center>
                                                <table><tr><td width="600">
                                            <![endif]-->
                                            <table width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%; max-width:600px;" align="center">
                                                <tr>
                                                    <td role="modules-container" style="padding:0px 0px 0px 0px; color:#516775; text-align:left;" bgcolor="#F9F5F2" width="100%" align="left"><table class="module preheader preheader-hide" role="module" data-type="preheader" border="0" cellpadding="0" cellspacing="0" width="100%" style="display: none !important; mso-hide: all; visibility: hidden; opacity: 0; color: transparent; height: 0; width: 0;">
                                                            <tr>
                                                                <td role="module-content">
                                                                    <p>{{.PreHeader}}</p>
                                                                </td>
                                                            </tr>
                                                        </table>
                                                        {{if .Logo}}
                                                        <table class="wrapper" role="module" data-type="image" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="qa8oMphYHuL7xyQrTVscgD">
                                                            <tbody><tr>
                                                                <td style="font-size:6px; line-height:10px; padding:30px 0px 20px 30px;" valign="top" align="left">
                                                                    <img class="max-width" border="0" style="display:block; color:#000000; text-decoration:none; font-family:Helvetica, arial, sans-serif; font-size:16px; max-width:30% !important; width:30%; height:auto !important;" src="{{.Logo}}" alt="logo" width="180" data-responsive="true" data-proportionally-constrained="false">
                                                                </td>
                                                            </tr>
                                                            </tbody></table>
                                                        {{end}}
                                                        <table class="module" role="module" data-type="text" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="bA2FfEE6abadx6yKoMr3F9" data-mc-module-version="2019-10-22">
                                                            <tbody><tr>
                                                                <td style="background-color:#ffffff; padding:40px 40px 50px 40px; line-height:22px; text-align:inherit;" height="100%" valign="top" bgcolor="#ffffff"><div><div style="font-family: inherit; text-align: inherit"><span style="color: #516775; font-family: georgia, serif; font-size: 28px; font-style: normal; font-variant-ligatures: normal; font-variant-caps: normal; font-weight: 700; letter-spacing: normal; orphans: 2; text-align: left; text-indent: 0px; text-transform: none; white-space: pre-wrap; widows: 2; word-spacing: 0px; -webkit-text-stroke-width: 0px; background-color: rgb(255, 255, 255); text-decoration-style: initial; text-decoration-color: initial; float: none; display: inline">{{.Title}}</span></div>
                                                                        <div style="font-family: inherit; text-align: inherit"><br></div>
                                                                        <div style="font-family: inherit; text-align: inherit"><span style="font-family: verdana, geneva, sans-serif">{{range .Body}}{{.}}{{end}}</span></div><div></div></div></td>
                                                            </tr>
                                                            </tbody></table><table class="module" role="module" data-type="text" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="298f3f34-bbc8-4654-999b-fb49b0ba2e5a" data-mc-module-version="2019-10-22">
                                                            <tbody>
                                                            <tr>
                                                                <td style="padding:18px 20px 18px 20px; line-height:20px; text-align:inherit; background-color:#F9F5F2;" height="100%" valign="top" bgcolor="#F9F5F2" role="module-content"><div><div style="font-family: inherit; text-align: inherit"><span style="font-family: arial, helvetica, sans-serif; font-size: 12px">Made with ♥ by Blue Giraffe Software</span></div><div></div></div></td>
                                                            </tr>
                                                            </tbody>
                                                        </table><table class="module" role="module" data-type="spacer" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="f5F8P1n4pQyU8o7DNMMEyW">
                                                            <tbody><tr>
                                                                <td style="padding:0px 0px 30px 0px;" role="module-content" bgcolor="">
                                                                </td>
                                                            </tr>
                                                            </tbody></table></td>
                                                </tr>
                                            </table>
                                            <!--[if mso]>
                                            </td>
                                            </tr>
                                            </table>
                                            </center>
                                            <![endif]-->
                                        </td>
                                    </tr>
                                </table>
                            </td>
                        </tr>
                    </table>
                </td>
            </tr>
        </table>
    </div>
</center>
</body>
</html>`