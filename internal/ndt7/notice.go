package ndt7

// URLs fijas de las políticas de privacidad.
const (
	MLabPrivacyURL  = "https://www.measurementlab.net/privacy/"
	NPerfPrivacyURL = "https://github.com/scouter190/Nperf/blob/main/docs/privacidad.md"
)

// Textos fijos del aviso de M-Lab (UI, CLI e informe).
const (
	// NoticeTitle encabeza el diálogo o mensaje de consentimiento.
	NoticeTitle = "Prueba de velocidad con M-Lab"
	// NoticeIP es el aviso principal: M-Lab publica la IP pública.
	NoticeIP = "Esta prueba envía tráfico a un servidor de M-Lab (Measurement Lab). " +
		"M-Lab publica la dirección IP pública de quien mide, la fecha y hora y los " +
		"metadatos de la prueba como datos abiertos (CC0), y los conserva de forma indefinida."
	// NoticeData avisa del consumo de datos.
	NoticeData = "La prueba descarga y sube datos durante unos 20 s: puede consumir cientos de MB " +
		"en conexiones rápidas; tenlo en cuenta en conexiones móviles o con tarifa por datos."
	// NoticeLimit informa del límite diario.
	NoticeLimit = "Solo se hace cuando tú lo pides; el límite es de 40 pruebas al día por usuario de Windows en este equipo."
	// Attribution es la atribución a M-Lab.
	Attribution = "Velocidad medida con NDT7 de M-Lab (Measurement Lab), https://www.measurementlab.net/"
)

// Notice devuelve el aviso completo (con las dos políticas enlazadas).
func Notice() string {
	return NoticeTitle + "\n" + NoticeIP + "\n" + NoticeData + "\n" + NoticeLimit + "\n" +
		"Política de M-Lab: " + MLabPrivacyURL + "\n" +
		"Política de NPerf: " + NPerfPrivacyURL
}
