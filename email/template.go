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
<center class="wrapper" data-link-color="#993300" data-body-style="font-size:16px; font-family:verdana,geneva,sans-serif; color:#516775; background-color:#f9f9f9;">
    <div class="webkit">
        <table cellpadding="0" cellspacing="0" border="0" width="100%" class="wrapper" bgcolor="#f9f9f9">
            <tr>
                <td valign="top" bgcolor="#f9f9f9" width="100%">
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
                                            <table width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%; {{if .FullWidth}}padding-left: 10px; padding-right: 10px;{{else}}max-width:600px;{{end}}" align="center">
                                                <tr>
                                                    <td role="modules-container" style="padding:0px 0px 0px 0px; color:#516775; text-align:left;" bgcolor="#ffffff" width="100%" align="left"><table class="module preheader preheader-hide" role="module" data-type="preheader" border="0" cellpadding="0" cellspacing="0" width="100%" style="display: none !important; mso-hide: all; visibility: hidden; opacity: 0; color: transparent; height: 0; width: 0;">
                                                            <tr>
                                                                <td role="module-content">
                                                                    <p>{{.PreHeader}}</p>
                                                                </td>
                                                            </tr>
                                                        </table><table class="module" role="module" data-type="spacer" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="a158f41c-2de0-4ba1-b8a3-7551e7716e33">
                                                            <tbody>
                                                            <tr>
                                                                <td style="padding:0px 0px 30px 0px;" role="module-content" bgcolor="#F9F9F9">
                                                                </td>
                                                            </tr>
                                                            </tbody>
                                                        </table><table class="wrapper" role="module" data-type="image" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="qa8oMphYHuL7xyQrTVscgD">
                                                            <tbody><tr>
                                                                <td style="font-size:6px; line-height:10px; padding:30px 0px 20px 30px;" valign="top" align="left">
                                                                    <img class="max-width" border="0" style="display:block; color:#000000; text-decoration:none; font-family:Helvetica, arial, sans-serif; font-size:16px; max-width:30% !important; height:auto !important;" src="{{.Logo}}" alt="Logo Image" width="180" data-responsive="true" data-proportionally-constrained="false">
                                                                </td>
                                                            </tr>
                                                            </tbody></table><table class="module" role="module" data-type="divider" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="66c75225-263a-42da-9476-dfe980e45d26">
                                                            <tbody>
                                                            <tr>
                                                                <td style="padding:0px 15px 0px 15px;" role="module-content" height="100%" valign="top" bgcolor="">
                                                                    <table border="0" cellpadding="0" cellspacing="0" align="center" width="100%" height="1px" style="line-height:1px; font-size:1px;">
                                                                        <tbody>
                                                                        <tr>
                                                                            <td style="padding:0px 0px 1px 0px;" bgcolor="#dddddd"></td>
                                                                        </tr>
                                                                        </tbody>
                                                                    </table>
                                                                </td>
                                                            </tr>
                                                            </tbody>
                                                        </table>
                                                        <table class="module" role="module" data-type="text" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="bA2FfEE6abadx6yKoMr3F9" data-mc-module-version="2019-10-22">
                                                            <tbody><tr>
                                                                <td style="background-color:#ffffff; padding:40px 40px 10px 40px; line-height:22px; text-align:inherit;" height="100%" valign="top" bgcolor="#ffffff"><div><div style="font-family: inherit; text-align: inherit"><span style="color: #516775; font-size: 28px; line-height: 28px; font-style: normal; font-variant-ligatures: normal; font-variant-caps: normal; font-weight: 700; letter-spacing: normal; orphans: 2; text-align: left; text-indent: 0px; text-transform: none; white-space: pre-wrap; widows: 2; word-spacing: 0px; -webkit-text-stroke-width: 0px; background-color: rgb(255, 255, 255); text-decoration-style: initial; text-decoration-color: initial; float: none; display: inline; font-family: arial,helvetica,sans-serif">{{.Title}}</span></div><div></div></div></td>
                                                            </tr>
                                                            </tbody></table>
                                                        {{range .Sections}}
                                                        {{if eq .Type "button"}}
															{{if eq .Button.Variant "outlined"}}
																<table border="0" cellpadding="0" cellspacing="0" class="module" data-role="module-button" data-type="button" role="module" style="table-layout:fixed;" width="100%" data-muid="9f229388-3506-4b95-bbdd-c48b0a8de0cb">
																	<tbody>
																	<tr>
																		<td align="left" bgcolor="" class="outer-td" style="padding:5px 0px 5px {{.PaddingLeft}};">
																			<table border="0" cellpadding="0" cellspacing="0" class="wrapper-mobile" style="text-align:center;">
																				<tbody>
																				<tr>
																					<td align="center" bgcolor="#ffffff" class="inner-td" style="border-radius:6px; font-size:16px; text-align:left; background-color:inherit;">
																						<a href="{{.Button.URL}}" style="background-color:#ffffff; border:1px solid #333333; border-color:#333333; border-radius:6px; border-width:1px; color:#333333; display:inline-block; font-size:14px; font-weight:normal; letter-spacing:0px; line-height:normal; padding:12px 18px 12px 18px; text-align:center; text-decoration:none; border-style:solid;" target="_blank">{{.Button.Text}}</a>
																					</td>
																				</tr>
																				</tbody>
																			</table>
																		</td>
																	</tr>
																	</tbody>
																</table>
															{{else}}
																<table border="0" cellpadding="0" cellspacing="0" class="module" data-role="module-button" data-type="button" role="module" style="table-layout:fixed;" width="100%" data-muid="9f229388-3506-4b95-bbdd-c48b0a8de0cb">
																	<tbody>
																	<tr>
																		<td align="left" bgcolor="" class="outer-td" style="padding:5px 0px 5px {{.PaddingLeft}};">
																			<table border="0" cellpadding="0" cellspacing="0" class="wrapper-mobile" style="text-align:center;">
																				<tbody>
																				<tr>
																					<td align="center" bgcolor="#333333" class="inner-td" style="border-radius:6px; font-size:16px; text-align:left; background-color:inherit;">
																						<a href="{{.Button.URL}}" style="background-color:#333333; border:1px solid #333333; border-color:#333333; border-radius:6px; border-width:1px; color:#ffffff; display:inline-block; font-size:14px; font-weight:normal; letter-spacing:0px; line-height:normal; padding:12px 18px 12px 18px; text-align:center; text-decoration:none; border-style:solid;" target="_blank">{{.Button.Text}}</a>
																					</td>
																				</tr>
																				</tbody>
																			</table>
																		</td>
																	</tr>
																	</tbody>
																</table>
                                                        	{{end}}
                                                        {{else if eq .Type "big-button"}}
                                                            <table border="0" cellpadding="0" cellspacing="0" class="module" data-role="module-button" data-type="button" role="module" style="table-layout:fixed;" width="100%" data-muid="9f229388-3506-4b95-bbdd-c48b0a8de0cb">
                                                                <tbody>
                                                                <tr>
                                                                    <td align="center" bgcolor="" class="outer-td" style="padding:5px 0px 5px 0px;">
                                                                        <table border="0" cellpadding="0" cellspacing="0" class="wrapper-mobile" style="text-align:center;">
                                                                            <tbody>
                                                                            <tr>
                                                                                <td align="center" bgcolor="#333333" class="inner-td" style="border-radius:6px; font-size:16px; text-align:center; background-color:inherit;">
                                                                                    <a href="{{.Button.URL}}" style="background-color:#333333; border:1px solid #333333; border-color:#333333; border-radius:6px; border-width:1px; color:#ffffff; display:inline-block; font-size:18px; font-weight:normal; letter-spacing:0px; line-height:normal; padding:12px 18px 12px 18px; text-align:center; text-decoration:none; border-style:solid;" target="_blank">{{.Button.Text}}</a>
                                                                                </td>
                                                                            </tr>
                                                                            </tbody>
                                                                        </table>
                                                                    </td>
                                                                </tr>
                                                                </tbody>
                                                            </table>
														{{else if eq .Type "flex-row-start"}}
															<table border="0" cellpadding="0" cellspacing="0" class="module" data-role="module-button" data-type="button" role="module" style="table-layout:fixed;" width="100%" data-muid="9f229388-3506-4b95-bbdd-c48b0a8de0cb">
															<tbody>
															<tr>
														{{else if eq .Type "flex-row-end"}}
															 <td></td><!-- push columns toward start of row -->
															</tr>
															</tbody>
															</table>
														{{else if eq .Type "flex-item-start"}}
															<td style="width: {{.Width}}; padding-left: {{.PaddingLeft}}">
														{{else if eq .Type "flex-item-end"}}
															</td>
                                                        {{else}}
                                                        <table class="module" role="module" data-type="text" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="9dee0372-e8be-49a8-b196-353c0ce6f623">
                                                            <tbody>
                                                            <tr>
                                                                <td style="padding:5px 40px 5px 40px; line-height:22px; text-align:inherit;" height="100%" valign="top" bgcolor="" role="module-content"><div><div style="font-family: inherit"><span style="color: #516775; font-family: arial, helvetica, sans-serif; font-size: 16px; font-style: normal; font-variant-ligatures: normal; font-variant-caps: normal; font-weight: 400; letter-spacing: normal; orphans: 2; text-align: start; text-indent: 0px; text-transform: none; white-space: pre-wrap; widows: 2; word-spacing: 0px; -webkit-text-stroke-width: 0px; background-color: rgb(255, 255, 255); text-decoration-style: initial; text-decoration-color: initial; float: none; display: inline">{{.HTML}}</span></div><div></div></div></td>
                                                            </tr>
                                                            </tbody>
                                                        </table>
                                                        {{end}}
                                                        {{end}}
                                                        <table class="module" role="module" data-type="spacer" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="097d6b3a-daf2-4922-9d57-a712eca50e0a">
                                                            <tbody>
                                                            <tr>
                                                                <td style="padding:0px 0px 30px 0px;" role="module-content" bgcolor="">
                                                                </td>
                                                            </tr>
                                                            </tbody>
                                                        </table><table class="module" role="module" data-type="text" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="298f3f34-bbc8-4654-999b-fb49b0ba2e5a" data-mc-module-version="2019-10-22">
                                                            <tbody>
                                                            <tr>
                                                                <td style="padding:18px 20px 18px 41px; line-height:20px; text-align:inherit; background-color:#F9F9F9;" height="100%" valign="top" bgcolor="#F9F9F9" role="module-content"><div>
                                                                        {{range .ContactAddress}}
                                                                        <div style="font-family: inherit; text-align: inherit"><span style="font-size: 12px">{{.}}</span></div>
                                                                        {{end}}
                                                                        <div style="font-family: inherit; text-align: inherit"><br></div>
                                                                        <div style="font-family: inherit"><span style="font-family: arial, helvetica, sans-serif; font-size: 12px">Made with ♥ by Blue Giraffe Software</span></div><div></div></div></td>
                                                            </tr>
                                                            </tbody>
                                                        </table><table class="module" role="module" data-type="spacer" border="0" cellpadding="0" cellspacing="0" width="100%" style="table-layout: fixed;" data-muid="7596c53e-dc62-4c91-a852-87c306e47b49">
                                                            <tbody>
                                                            <tr>
                                                                <td style="padding:0px 0px 30px 0px;" role="module-content" bgcolor="#F9F9F9">
                                                                </td>
                                                            </tr>
                                                            </tbody>
                                                        </table></td>
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
