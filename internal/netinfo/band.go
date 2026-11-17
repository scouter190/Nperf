package netinfo

// BandFromChannel deduce banda y canal. Reglas, por orden:
//  1. Si hay frecuencia del BSS (freqKHz > 0) y es reconocible, manda ella
//     (BandChannel); el canal indicado se ignora.
//  2. Frecuencia >= 5925 MHz con canal y PHY HE/EHT: 6 GHz con ese canal.
//  3. Solo canal: 1-14 -> 2.4 GHz; 32-177 -> 5 GHz; 178-233 -> 6 GHz solo con
//     PHY HE/EHT (no existen en 5 GHz). Los canales 1-233 de 6 GHz que
//     solapan con 2.4/5 GHz son ambiguos sin frecuencia: se prefiere la banda
//     de 2.4/5 GHz, nunca 6 GHz sin indicio.
//  4. Sin datos o fuera de rango: ok=false ("no disponible").
func BandFromChannel(channel, phyType, freqKHz int) (band string, ch int, ok bool) {
	if freqKHz > 0 {
		if b, c, ok := BandChannel(freqKHz); ok {
			return b, c, true
		}
		if freqKHz >= 5925000 && channel > 0 && channel <= 233 && (phyType == phyHE || phyType == phyEHT) {
			return Band6, channel, true
		}
	}
	switch {
	case channel >= 1 && channel <= 14:
		return Band24, channel, true
	case channel >= 32 && channel <= 177:
		return Band5, channel, true
	case channel >= 178 && channel <= 233 && (phyType == phyHE || phyType == phyEHT):
		return Band6, channel, true
	}
	return "", 0, false
}
